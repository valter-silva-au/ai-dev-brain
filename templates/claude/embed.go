package claudetpl

import "embed"

//go:embed skills agents hooks rules claudeignore.template settings.template.json mcp-servers.json
var FS embed.FS
