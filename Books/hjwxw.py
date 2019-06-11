import urllib.request
import os
import zipfile
import sqlite3
import http
import gzip
import io
import ClassDefinition

class HJWXW():
    def __init__(self, dbConn, path):
        self._webpage = ""
        self._bookNum = 0
        self.books = []
        self._conn = dbConn
        self._cursor = self._conn.cursor()
        self._path = path
    class Book(ClassDefinition.BaseBook):
        def _getBasicInfo(self):
            # fill back the info by the website
            try:
                res = urllib.request.urlopen(self._website)
                content = res.read()

                # decode the content
                if (res.info().get('Content-Encoding') == 'gzip'):
                    gzipFile = gzip.GzipFile('','rb',9,io.BytesIO(content))
                    content = gzipFile.read()
                content = content.decode("utf-8")

            # return false if the webpage is not exist or not avaliable
            except urllib.error.HTTPError:
                return False

            if(not self._name):
                # get name
                start = content.find("<title>")
                self._name = content[start+7:]
                end = self._name.find("/")
                self._name = self._name[:end]
                self._updated = True
            if(not self._writer):
                # get writer (writer)
                start = content.find("作者標簽:")
                self._writer = content[start+4:]
                start = self._writer.find('</a>')
                self._writer = self._writer[start+1:]
                end = self._writer.find('</a>')
                self._writer = self._writer[:end]
                self._updated = True

            # get date (always get)
            start = content.find('更新时间: ')
            date = content[start+6:]
            end = date.find('">')
            date = date[:end]
            if(self._date != date):
                self._date = date
                self._updated = True

            # get chapter (always get)
            start = content.find("章節名:")
            chapter = content[start+4:]
            end = chapter.find('更新时间')
            chapter = chapter[:end]
            if(self._chapter != chapter):
                self._chapter = chapter
                self._updated = True
            if(not self._bookType):
                # check type (bookType)
                bookType = ""
                c = content
                start = c.find('小說分類標簽: ')
                if(start>0):
                    c = content[start+8:]
                    end = c.find('  ')
                    bookType += c[:end]
                    c = c[end:]
                    start = c.find('小說分類標簽: ')                    
                self._updated = True
                self._bookType = bookType
            return self._updated
        def DownloadBook(self,path,out=print):
            # fill back the info by the website
            res = urllib.request.urlopen(self._website)
            content = res.read()
            # decode the content
            if (res.info().get('Content-Encoding') == 'gzip'):
                gzipFile = gzip.GzipFile('','rb',9,io.BytesIO(content))
                content = gzipFile.read()
            content = content.decode("utf-8")

            # TODO #
            # get chapter set
            start = content.find("yulan")
            chapters = content[start:]
            end = chapters.find("</div>")
            chapters = chapters[:end]
            self._chapterSet = chapters.split("href=")
            out(self._name)
            # download chapters one by one
            for chapter in self._chapterSet:
                if("<B>" in chapter):
                    self._text += chapter[chapter.find("<B>")+3:chapter.find("<a")]+"\n"
                elif("</B>" not in chapter)and("https" in chapter):
                    # go to the website and download
                    chapter = chapter[chapter.find('"')+1:]
                    chapter = chapter[:chapter.find('"')]
                    self._DownloadChapter(chapter)
                    # log for progress
                    out("\r"+self._chapter,end=" "*20)
            # save it into file
            try: os.mkdir(path)
            except: pass
            try: os.mkdir(path+"\\"+self._bookType)
            except: pass
            f = open(path+"\\"+self._bookType+"\\"+self._name+"-"+self._writer+".txt","w",encoding='utf8')
            f.write(self._text)
            f.close()
            self._cursor.execute("update books set download='true' where website='"+book._website+"'")
            # TODO END #
        def _DownloadChapter(self,url):
            # open chapter url
            chRes = urllib.request.urlopen(url)
            content = chRes.read()
            # decode the content
            if (chRes.info().get('Content-Encoding') == 'gzip'):
                gzipFile = gzip.GzipFile('','rb',9,io.BytesIO(content))
                content = gzipFile.read()
            content = content.decode("utf-8")
            # TODO #
            # get the title
            self._text += "\n"
            start = content.find("<h1>")
            title = content[start+4:]
            end = title.find("</h1>")
            title = title[:end]
            self._chapter = title
            self._text += title + "\n"
            # get the content
            start = content.find('id="content">')
            c = content[start+13:]
            end = c.find("<div")
            c = c[:end]
            c = c.replace("&nbsp;"," ")
            c = c.replace("<br />    ","\n")
            self._text += c
            # TODO END #
    def Download(self,out=print):
        # put [end] book in db to books
        self.books.clear()
        for row in self._cursor.execute("select * from books where end='true' and download='false' and website like '%hjwxw%'"):
            self.books.append(self.Book(row[4],name=row[0],writer=row[1],date=row[2],chapter=row[3],bookType=row[5]))
        out("downloading")
        for book in self.books:
            book.DownloadBook(self._path)
        out("finish download")
    def Update(self,out=print):
        # get all books from db to boo
        self.books.clear()
        for row in self._cursor.execute("select * from books where website like '%hjwxw%'"):
            self.books.append(self.Book(row[4],name=row[0],writer=row[1],date=row[2],chapter=row[3],bookType=row[5]))
        # check any update
        out("updating")
        for book in self.books:
            # if the book info had been updated
            if(book._updated):
                sql = "update books set date='"+book._date+"', chapter='"+book._chapter+"' where website='"+book._website+"'"
                self._cursor.execute(sql)
                self._conn.commit()
                # if it update, but ended, add a '-' at first of the file name
                flag = self._cursor.execute("select download from books where website='"+book._website+"'").fetchone()
                for f in flag:
                    if(f=="false"):
                        f = False
                        break
                if(f):
                    os.rename(self._path+"\\"+book._bookType+"\\"+book._name+"-"+book._writer+".txt",self._path+"\\"+book._bookType+"\\-"+book._name+"-"+book._writer+".txt")
                    book._name = '-'+book._name
                    self._cursor.execute("update books set name='"+book._name+"' where website='"+book._website+"'")
                    self._conn.commit()
        out("update finish")
    def Explore(self,n,out=print):
        # get the max book num from the db
        self.books.clear()
        self._bookNum = 12343
        for row in self._cursor.execute("select website from books where website like '%hjwxw%' order by website desc"):
            i = row[0]
            i = int(i[i.find("_")+1:i.rfind(".")])
            if(i > self._bookNum):
                self._bookNum = i
        self._bookNum += 1
        errorPage = 0
        # TODO #
        # check any new book by the book num
        while(errorPage<n):
            b = self.Book("https://www.80txt.com/txtml_"+str(self._bookNum)+".html")
            if(b._name):
                self.books.append(b)
                out("r"+b._name+"\t"=._bookType,end=" "*10)
                errorPage = 0
            else: errorPage += 1
            self._bookNum += 1
        # TODO END #
        # save the book num
        for book in self.books:
            flag = bool(self._cursor.execute("select * from books where name='"+book._name+"' and writer='"book._writer+"'").fetchone())
            if(not(flag)):
                sql = (
                    "insert into books (name,writer,date,chapter,website,type,download) values"
                    "('"+book._name+"','"+book._writer+"','"+book._date+"','"+book._chapter+"','"+book._website+"','"+book._bookType+"','false')"
                )
                self._cursor.execute(sql)
                self._conn.commit()
