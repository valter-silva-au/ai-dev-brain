package claudetpl

import "embed"

//go:embed skills agents hooks rules checklists artifacts claudeignore.template settings.template.json mcp-servers.json
var FS embed.FS
