package model

import (
	"fmt"
	"io/ioutil"
	"encoding/json"
	"golang.org/x/text/encoding"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"context"
	"golang.org/x/sync/semaphore"
	"sync"
	"strconv"
	"../helper"
	//"time"
	//"runtime"
)

var SITE_MAX_THREAD int = 1000;

type Site struct {
	SiteName string
	database *sql.DB
	MetaBaseUrl, metaDownloadUrl, metaChapterUrl, chapterPattern string
	decoder *encoding.Decoder
	titleRegex, writerRegex, typeRegex, lastUpdateRegex, lastChapterRegex string
	chapterUrlRegex, chapterTitleRegex string
	chapterContentRegex string
	downloadLocation string
	bookTx *sql.Tx
}

func NewSite(siteName string, decoder *encoding.Decoder, configFileLocation string, databaseLocation string, downloadLocation string) (Site) {
	database, err := sql.Open("sqlite3", databaseLocation)
	helper.CheckError(err);
	database.SetMaxIdleConns(10);
	database.SetMaxOpenConns(99999);
	data, err := ioutil.ReadFile(configFileLocation)
	helper.CheckError(err);
	var info map[string]interface{};
	if err = json.Unmarshal(data, &info); err != nil {
        panic(err);
	}
	site := Site{
		SiteName: siteName,
		database: database,
		MetaBaseUrl: info["metaBaseUrl"].(string),
		metaDownloadUrl: info["metaDownloadUrl"].(string),
		metaChapterUrl: info["metaChapterUrl"].(string),
		chapterPattern: info["chapterPattern"].(string),
		decoder: decoder,
		titleRegex: info["titleRegex"].(string),
		writerRegex: info["writerRegex"].(string),
		typeRegex: info["typeRegex"].(string),
		lastUpdateRegex: info["lastUpdateRegex"].(string),
		lastChapterRegex: info["lastChapterRegex"].(string),
		chapterUrlRegex: info["chapterUrlRegex"].(string),
		chapterTitleRegex: info["chapterTitleRegex"].(string),
		chapterContentRegex: info["chapterContentRegex"].(string),
		downloadLocation: downloadLocation};
	return site;
}

func (site *Site) Book(id int) (Book) {
	baseUrl := fmt.Sprintf(site.MetaBaseUrl, id);
	downloadUrl := fmt.Sprintf(site.metaDownloadUrl, id);
	var siteName string;
	var temp int;
	version := -1;
	title := "";
	writer := "";
	typeName := "";
	lastUpdate := "";
	lastChapter := "";
	end := false;
	download := false;
	read := false;
	for i := 0; i < 10; i++ {
		rows, err := site.bookTx.Query("select site, num, version, name, writer, "+
						"type, date, chapter, end, download, read from books where "+
						"num="+strconv.Itoa(id) +
						" order by version desc");
		if (err != nil) {
			fmt.Println(id)
			fmt.Println(err)
			//time.Sleep(1000)
			continue
		}
		if (rows.Next()) {
			rows.Scan(&siteName, &temp, &version, &title, &writer, &typeName,
						&lastUpdate, &lastChapter, &end, &download, &read);
		} else {
			fmt.Println("retry (" + strconv.Itoa(i) + ") Cannot load " + strconv.Itoa(id) + " from database")
			//time.Sleep(1000)
			continue
		}
		rows.Close()
		break
		//panic(err)
	}
	book := Book{
		SiteName: site.SiteName,
		Id: id,
		Version: version,
		Title: title,
		Writer: writer,
		Type : typeName,
		LastUpdate: lastUpdate,
		LastChapter: lastChapter,
		EndFlag: end,
		DownloadFlag: download,
		ReadFlag: read,
		decoder: site.decoder,
		baseUrl: baseUrl,
		downloadUrl: downloadUrl,
		chapterUrl: site.metaChapterUrl,
		chapterPattern: site.chapterPattern,
		titleRegex: site.titleRegex,
		writerRegex: site.writerRegex,
		typeRegex: site.typeRegex,
		lastUpdateRegex: site.lastUpdateRegex,
		lastChapterRegex: site.lastChapterRegex,
		chapterUrlRegex: site.chapterUrlRegex,
		chapterTitleRegex: site.chapterTitleRegex,
		chapterContentRegex: site.chapterContentRegex};
	return book;
}

func (site *Site) Update() () {
	// init concurrent variable
	ctx := context.Background()
	site.bookTx, _ = site.database.Begin()
	var s = semaphore.NewWeighted(int64(SITE_MAX_THREAD))
	var wg sync.WaitGroup
	var siteName string;
	var id int;
	// prepare transaction and statements
	tx, err := site.database.Begin()
	helper.CheckError(err)
	save, err := site.database.Prepare("insert into books "+
					"(site, num, version, name, writer, type, date, chapter, end, download, read)"+
					" values "+
					"(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)");
	helper.CheckError(err);
	defer save.Close()
	update, err := site.database.Prepare("update books set version=?, name=?, writer=?, type=?,"+
					"date=?, chapter=?, end=?, download=?, read=? where site=? and num=?");
	helper.CheckError(err);
	defer update.Close()
	// update all normal books
	rows, _ := site.database.Query("SELECT site, num FROM books order by date desc");
	i := 0;
	for rows.Next() {
		wg.Add(1)
		s.Acquire(ctx, 1);
		rows.Scan(&siteName, &id);
		//book := site.Book(id)
		go site.updateThread(id, s, &wg, tx, save, update);
		if (i % 100 == 0) {
			helper.CheckError(err);
		}
		i++;
	}
	rows.Close()
	wg.Wait()
	tx.Commit()
}
func (site *Site) updateThread(id int, s *semaphore.Weighted, wg *sync.WaitGroup, tx *sql.Tx, save *sql.Stmt, update *sql.Stmt) () {
	defer wg.Done()
	defer s.Release(1)
	book := site.Book(id)
	checkVersion := book.Version;
	// try to update book
	updated := book.Update();
	if (updated) {
		// if version different, save a new record
		if (book.Version != checkVersion) {
			tx.Stmt(save).Exec(site.SiteName, book.Id, book.Version,
						book.Title, book.Writer, book.Type,
						book.LastUpdate, book.LastChapter,
						book.EndFlag, book.DownloadFlag, book.ReadFlag);
			fmt.Println("new version update " + strconv.Itoa(checkVersion) + " -> " + strconv.Itoa(book.Version) + " - - - - - - - -\n"+book.String());
			fmt.Println();
		} else { // update old record
			tx.Stmt(update).Exec(book.Version, book.Title, book.Writer, book.Type,
						book.LastUpdate, book.LastChapter,
						book.EndFlag, book.DownloadFlag, book.ReadFlag,
						book.SiteName, book.Id);
			fmt.Println("regular version update - - - - - - - - - -\n"+book.String());
			fmt.Println();
		}
	} else {
		// tell others nothing updated
		fmt.Println("Not updated - - - - - - - - - - - - - - -\n" + book.String())
		fmt.Println()
	}
}

func (site *Site) Explore(maxError int) () {
	// init concurrent variable
	ctx := context.Background()
	site.bookTx, _ = site.database.Begin()
	var s = semaphore.NewWeighted(int64(SITE_MAX_THREAD))
	var wg sync.WaitGroup
	// prepare transaction and statement
	tx, _ := site.database.Begin();
	save, err := site.database.Prepare("insert into books "+
					"(site, num, version, name, writer, type, date, chapter, end, download, read)"+
					" values "+
					"(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)");
	helper.CheckError(err);
	defer save.Close();
	saveError, err := site.database.Prepare("insert into error "+
					"(site, num)"+
					" values "+
					"(?, ?)");
	helper.CheckError(err);
	defer saveError.Close();
	deleteError, err := site.database.Prepare("delete from books "+
					"where site=? and num=?");
	helper.CheckError(err);
	defer deleteError.Close();
	// find max id
	rows, err := site.database.Query("select site, num from books order by num desc");
	helper.CheckError(err)
	var siteName string
	var maxId int;
	if (rows.Next()) {
		rows.Scan(&siteName, &maxId);
		maxId++;
	} else {
		maxId = 1;
	}
	fmt.Println(maxId);
	// keep explore until reach max error count
	errorCount := 0
	for (errorCount < maxError) {
		wg.Add(1)
		s.Acquire(ctx, 1);
		//book := site.Book(maxId);
		go site.exploreThread(maxId, &errorCount, s, &wg, tx, save, saveError, deleteError);
		maxId++;
	}
	tx.Commit()
	rows.Close();
	wg.Wait()
}
func (site *Site) exploreThread(id int, errorCount *int, s *semaphore.Weighted, wg *sync.WaitGroup, tx *sql.Tx, save, saveError, deleteError *sql.Stmt) () {
	defer wg.Done()
	defer s.Release(1)
	book := site.Book(id)
	updated := book.Update();
	// if updated, save in books table, else, save in error table and **reset error count**
	if (updated) {
		tx.Stmt(save).Exec(book.SiteName, book.Id, book.Version,
					book.Title, book.Writer, book.Type,
					book.LastUpdate, book.LastChapter,
					book.EndFlag, book.DownloadFlag, book.ReadFlag);
		tx.Stmt(deleteError).Exec(book.SiteName, book.Id)
		fmt.Println("Explore - - - - - - - - - - - -\n" + book.String())
		fmt.Println();
		*errorCount = 0;
	} else { // increase error Count
		tx.Stmt(saveError).Exec(book.SiteName, book.Id)
		fmt.Println("Unreachable - - - - - - - - - - -\n" + book.String());
		fmt.Println();
		*errorCount++;
	}
}

func (site *Site) Download() () {
	rows, _ := site.database.Query("select num from books where end=true and download=false")
	var id int;
	if (rows.Next()) {
		rows.Scan(&id);
		book := site.Book(id);
		check := book.Download(site.downloadLocation)
		if (! check) {
			fmt.Println("download failure\t" + strconv.Itoa(book.Id) + "\t" + book.Title)
		}
	}
}

func (site *Site) UpdateError() () {
	// init concurrent variable
	ctx := context.Background()
	var s = semaphore.NewWeighted(int64(SITE_MAX_THREAD))
	var wg sync.WaitGroup
	var siteName string;
	var id int;
	// prepare transaction and statements
	tx, _ := site.database.Begin()
	delete, err := site.database.Prepare("delete error where site=? and num=?");
	helper.CheckError(err);
	defer delete.Close()
	save, err := site.database.Prepare("insert into error "+
					"(site, num, version, name, writer, type, date, chapter, end, download, read)"+
					" values "+
					"(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)");
	helper.CheckError(err);
	defer save.Close()
	// try update all error books
	rows, _ := site.database.Query("SELECT site, num FROM error order by date desc");
	for rows.Next() {
		wg.Add(1)
		s.Acquire(ctx, 1);
		rows.Scan(&siteName, &id);
		//book := site.Book(id)
		go site.updateErrorThread(id, s, &wg, tx, delete, save);
	}
	rows.Close()
	wg.Wait()
}
func (site *Site) updateErrorThread(id int, s *semaphore.Weighted, wg *sync.WaitGroup, tx *sql.Tx, delete, save *sql.Stmt) () {
	defer wg.Done()
	defer s.Release(1)
	// try to update book
	book := site.Book(id)
	updated := book.Update();
	if (updated) {
		// if update successfully
		tx.Stmt(delete).Exec(site.SiteName, book.Id);
		tx.Stmt(save).Exec(site.SiteName, book.Id, book.Version,
					book.Title, book.Writer, book.Type,
					book.LastUpdate, book.LastChapter,
					book.EndFlag, book.DownloadFlag, book.ReadFlag);
		fmt.Println("Error update - - - - - - - - - -\n"+book.String());
		fmt.Println();
		tx.Commit()
	} else {
		// tell others nothing updated
		fmt.Println("Not updated - - - - - - - - - - -\n" + book.String())
		fmt.Println()
	}
}

func (site Site) Info() () {
	fmt.Println("Site :\t" + site.SiteName);
	var normalCount, errorCount int;
	var rows *sql.Rows;
	rows, _ = site.database.Query("select count(DISTINCT num) as c from books");
	for rows.Next() {
		rows.Scan(&normalCount);
	}
	rows.Close()
	fmt.Println("Normal Book Count :\t" + strconv.Itoa(normalCount));
	rows, _ = site.database.Query("select count(num) as c from error");
	for rows.Next() {
		rows.Scan(&errorCount);
	}
	rows.Close()
	fmt.Println("Error Book Count :\t" + strconv.Itoa(errorCount));
	fmt.Println("Total Book Count :\t" + strconv.Itoa(normalCount + errorCount));
	rows, _ = site.database.Query("select num from books order by num desc limit 1");
	for rows.Next() {
		rows.Scan(&normalCount);
	}
	rows.Close()
	rows, _ = site.database.Query("select num from error order by num desc limit 1");
	for rows.Next() {
		rows.Scan(&errorCount);
	}
	rows.Close()
	var max int;
	if (normalCount > errorCount) {
		max = normalCount;
	} else {
		max = errorCount;
	}
	fmt.Println("Max Book Number :\t" + strconv.Itoa(max));
}

func (site *Site) FixStroageError() () {

}

func (site *Site) FixDatabaseError() () {

}

func (site *Site) Check() () {

}

func (site *Site) Backup() () {

}
