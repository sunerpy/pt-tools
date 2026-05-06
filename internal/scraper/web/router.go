package web

import "net/http"

func (a *API) RegisterRoutes(mux *http.ServeMux, authMw func(http.HandlerFunc) http.HandlerFunc) {
	if mux == nil {
		return
	}
	if authMw == nil {
		authMw = NoAuthMiddleware()
	}
	mux.HandleFunc("GET /api/v2/scraper/libraries", authMw(a.HandleListLibraries))
	mux.HandleFunc("POST /api/v2/scraper/libraries", authMw(a.HandleCreateLibrary))
	mux.HandleFunc("GET /api/v2/scraper/libraries/{id}", authMw(a.HandleGetLibrary))
	mux.HandleFunc("PUT /api/v2/scraper/libraries/{id}", authMw(a.HandleUpdateLibrary))
	mux.HandleFunc("DELETE /api/v2/scraper/libraries/{id}", authMw(a.HandleDeleteLibrary))
	mux.HandleFunc("POST /api/v2/scraper/scrape", authMw(a.HandleScrape))
	mux.HandleFunc("GET /api/v2/scraper/tasks", authMw(a.HandleListTasks))
	mux.HandleFunc("GET /api/v2/scraper/tasks/{id}", authMw(a.HandleGetTask))
	mux.HandleFunc("DELETE /api/v2/scraper/tasks/{id}", authMw(a.HandleDeleteTask))
	mux.HandleFunc("GET /api/v2/scraper/providers", authMw(a.HandleListProviders))
	mux.HandleFunc("POST /api/v2/scraper/providers/{name}/credentials", authMw(a.HandleSetProviderCredential))
	mux.HandleFunc("GET /api/v2/scraper/connectors", authMw(a.HandleListConnectors))
	mux.HandleFunc("POST /api/v2/scraper/connectors/{id}/test", authMw(a.HandleTestConnector))
	mux.HandleFunc("GET /api/v2/scraper/settings", authMw(a.HandleGetSettings))
	mux.HandleFunc("PUT /api/v2/scraper/settings", authMw(a.HandleGetSettings))
	mux.HandleFunc("POST /api/v2/scraper/search", authMw(a.HandleSearch))
	mux.HandleFunc("GET /api/v2/scraper/artworks", authMw(a.HandleGetArtworks))
	mux.HandleFunc("GET /api/v2/scraper/llm/providers", authMw(a.HandleListLLMProviders))
	mux.HandleFunc("POST /api/v2/scraper/llm/generate", authMw(a.HandleLLMGenerate))
	mux.HandleFunc("POST /api/v2/scraper/llm/validate", authMw(a.HandleLLMValidate))
}
