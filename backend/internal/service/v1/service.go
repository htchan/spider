package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	client "github.com/htchan/BookSpider/internal/client_v2"
	config "github.com/htchan/BookSpider/internal/config_new"
	"github.com/htchan/BookSpider/internal/model"
	"github.com/htchan/BookSpider/internal/repo"
	serv "github.com/htchan/BookSpider/internal/service"
	"github.com/htchan/BookSpider/internal/vendor"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/semaphore"
)

type ServiceImpl struct {
	name       string
	cli        client.BookClient
	rpo        repo.Repository
	parser     vendor.Parser
	urlBuilder vendor.BookURLBuilder

	conf config.SiteConfig
	sema *semaphore.Weighted
}

var _ serv.Service = (*ServiceImpl)(nil)

func NewService(
	name string, cli client.BookClient, rpo repo.Repository,
	parser vendor.Parser, urlBuilder vendor.BookURLBuilder,
) *ServiceImpl {
	return &ServiceImpl{
		name:       name,
		cli:        cli,
		rpo:        rpo,
		parser:     parser,
		urlBuilder: urlBuilder,
	}
}

func (s *ServiceImpl) Name() string {
	return s.name
}

func (s *ServiceImpl) bookFileLocation(bk *model.Book) string {
	filename := fmt.Sprintf("%d.txt", bk.ID)
	if bk.HashCode > 0 {
		filename = fmt.Sprintf("%d-v%s.txt", bk.ID, bk.FormatHashCode())
	}

	return filepath.Join(s.conf.Storage, filename)
}

func (s *ServiceImpl) checkBookStorage(bk *model.Book) bool {
	isDownloadUpdated, fileExist := false, true

	if _, err := os.Stat(s.bookFileLocation(bk)); err != nil {
		fileExist = false
	}

	if fileExist && !bk.IsDownloaded {
		log.Info().Str("book", bk.String()).Msg("file exist for not downloaded book")
		bk.IsDownloaded = true
		isDownloadUpdated = true
	} else if !fileExist && bk.IsDownloaded {
		log.Info().Str("book", bk.String()).Msg("file not exist for downloaded book")
		bk.IsDownloaded = false
		isDownloadUpdated = true
	}

	return isDownloadUpdated
}

func (s *ServiceImpl) PatchDownloadStatus(ctx context.Context) error {
	bks, err := s.rpo.FindAllBooks()
	if err != nil {
		return fmt.Errorf("patch download status fail: %w", err)
	}

	var wg sync.WaitGroup
	zerolog.Ctx(ctx).Info().Str("site", s.name).Msg("update books is_downloaded by storage")

	for bk := range bks {
		bk := bk
		s.sema.Acquire(ctx, 1)
		wg.Add(1)

		go func(bk *model.Book) {
			defer wg.Done()
			defer s.sema.Release(1)

			needUpdate := s.checkBookStorage(bk)
			if needUpdate {
				err := s.rpo.UpdateBook(bk)
				if err != nil {
					zerolog.Ctx(ctx).Error().Err(err).
						Str("site", s.name).
						Int("bk_id", bk.ID).
						Str("bk_hash_code", bk.FormatHashCode()).
						Msg("update book is_downloaded fail")
				}
			}
		}(&bk)
	}

	wg.Wait()

	return nil
}

func (s *ServiceImpl) PatchMissingRecords(ctx context.Context) error {
	zerolog.Ctx(ctx).Info().Msg("patch missing records")

	var wg sync.WaitGroup
	allBkIDs, err := s.rpo.FindAllBookIDs()
	if err != nil {
		return fmt.Errorf("find all book ids fail: %w", err)
	}

	missingIDs := s.parser.FindMissingIds(allBkIDs)
	for _, bookID := range missingIDs {
		bookID := bookID
		s.sema.Acquire(ctx, 1)
		wg.Add(1)

		go func(id int) {
			defer s.sema.Release(1)
			defer wg.Done()

			zerolog.Ctx(ctx).Error().Err(err).Int("id", id).Msg("book not exist in database")
			bk := model.NewBook(s.name, id)
			s.ExploreBook(ctx, &bk)
		}(bookID)
	}
	wg.Wait()

	return nil
}

func (s *ServiceImpl) CheckAvailability(ctx context.Context) error {
	body, err := s.cli.Get(ctx, s.urlBuilder.AvailabilityURL())
	if err != nil {
		return fmt.Errorf("get availability page failed: %w", err)
	}

	if !s.parser.IsAvailable(body) {
		return serv.ErrUnavailable
	}

	return nil
}