package docs

import (
	_ "embed"
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

//go:embed openapi.yaml
var openapiSpec []byte

func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi.yaml", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.Header().Set(
			"Content-Type",
			"application/yaml; charset=utf-8",
		)

		_, _ = w.Write(openapiSpec)
	})

	mux.Handle(
		"GET /swagger/",
		httpSwagger.Handler(
			httpSwagger.URL("/openapi.yaml"),
		),
	)
}
