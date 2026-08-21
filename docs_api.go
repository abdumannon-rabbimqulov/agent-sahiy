package main

import (
	_ "embed"
	"net/http"
)

// openapiSpec — API tavsifi binar ichiga qo'shib yuboriladi, ya'ni konteynerda
// qo'shimcha fayl kerak emas.
//
//go:embed openapi.json
var openapiSpec []byte

// swaggerUI — FastAPI'dagi /docs kabi sahifa. Swagger UI fayllari CDN'dan
// olinadi (brauzerda internet kerak), spec esa o'zimizning /openapi.json dan.
const swaggerUI = `<!doctype html>
<html lang="uz">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Sahiy lokal API — hujjat</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/openapi.json",
      dom_id: "#swagger-ui",
      deepLinking: true,
      tryItOutEnabled: true,
      displayRequestDuration: true,
      defaultModelsExpandDepth: 0
    });
  </script>
</body>
</html>`

// redocUI — o'qishga qulayroq muqobil ko'rinish (/redoc).
const redocUI = `<!doctype html>
<html lang="uz">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Sahiy lokal API — hujjat</title>
  <style>body { margin: 0; }</style>
</head>
<body>
  <redoc spec-url="/openapi.json"></redoc>
  <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
</body>
</html>`

// openapiHandler: GET /openapi.json — Postman/Insomnia'ga import qilsa bo'ladi.
func openapiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(openapiSpec)
}

// docsHandler: GET /docs — Swagger UI. "/" ham shu yerga yo'naltiriladi.
func docsHandler(w http.ResponseWriter, r *http.Request) {
	// http.HandleFunc("/") hamma noma'lum yo'lni ushlaydi — 404 ni o'zimiz beramiz.
	if r.URL.Path != "/docs" && r.URL.Path != "/" {
		http.Error(w, `{"error":"bunday yo'l yo'q — /docs ga qarang"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(swaggerUI))
}

// redocHandler: GET /redoc — o'sha spec, boshqa ko'rinishda.
func redocHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(redocUI))
}
