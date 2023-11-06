package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/htchan/BookSpider/internal/config/v2"
	"github.com/htchan/BookSpider/internal/service"
)

func AddLiteRoutes(router chi.Router, conf config.APIConfig, services map[string]service.Service) {
	router.Route(conf.LiteRoutePrefix, func(router chi.Router) {
		router.Use(SetUriPrefixMiddleware(conf.LiteRoutePrefix))

		router.Route("/sites/{siteName}", func(router chi.Router) {
			router.Use(ZerologMiddleware)
			router.Use(GetSiteMiddleware(services))
			router.Get("/", SiteLiteHandlerfunc)

			router.With(GetSearchParamsMiddleware).With(GetPageParamsMiddleware).Get("/search", SearchLiteHandler)
			router.With(GetPageParamsMiddleware).Get("/random", RandomLiteHandler)

			router.Route("/books", func(router chi.Router) {
				router.Route("/{idHash:\\d+(-[\\w]+)?}", func(router chi.Router) {
					// idHash format is <id>-<hash>
					router.Use(GetBookMiddleware)
					router.Get("/", BookLiteHandler)
					router.Get("/download", DownloadLiteHandler)
				})
			})
		})

		router.Get("/", GeneralLiteHandler(services))
	})
}
