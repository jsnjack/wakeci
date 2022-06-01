package main

import (
	"net/http"
)

// HandleAPIDocsView renders swagger UI
func HandleAPIDocsView(w http.ResponseWriter, r *http.Request) {
	indexDoc := `
<!DOCTYPE html>
<html lang="en">
<head>
	<title>wakeci - API</title>
	<script src="https://unpkg.com/swagger-ui-dist@4.4.0/swagger-ui-bundle.js"></script>
	<link href="https://unpkg.com/swagger-ui-dist@4.4.0/swagger-ui.css" rel="stylesheet">
</head>
<body>
	<div id="swagger-container"><div>
	<script>
		const ui = SwaggerUIBundle({
			url: "/docs/swagger.json",
			dom_id: "#swagger-container",
		})
	</script>
</body>
</html>
`
	w.Header().Set("content-security-policy", "default-src 'self'; script-src 'self' https://unpkg.com 'sha256-vRXqJZN28RidhIcwjq/UDO9cWHiPP1H4fdwi42L1TOM='; style-src 'self' https://unpkg.com; img-src 'self' data:; frame-ancestors 'self'")
	w.Write([]byte(indexDoc))
}
