package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var sb strings.Builder

	sb.WriteString(GetHead())
	sb.WriteString("<body>\n")
	sb.WriteString(GetLoginView())
	sb.WriteString(GetAppViewStart())
	sb.WriteString(GetSidebarDesktop())
	sb.WriteString(GetMobileDrawer())
	sb.WriteString(GetTopHeader())
	sb.WriteString("      <main class=\"flex-1 p-4 sm:p-6 lg:p-8 overflow-y-auto space-y-6\">\n")
	sb.WriteString(GetTabsCore())
	sb.WriteString(GetTabsFintech())
	sb.WriteString(GetTabsEnterprise())
	sb.WriteString("      </main>\n")
	sb.WriteString("    </div>\n")
	sb.WriteString("  </div>\n\n")
	sb.WriteString("  <!-- MODALS -->\n")
	sb.WriteString(GetModalsCore())
	sb.WriteString(GetModalsFintech())
	sb.WriteString(GetModalsEnterprise())
	sb.WriteString(GetToast())
	sb.WriteString("\n  <script>\n")

	coreJS, err := os.ReadFile(filepath.Join("cmd", "build_console", "scripts_core.js"))
	if err != nil {
		panic(err)
	}
	sb.Write(coreJS)
	sb.WriteString("\n\n")

	finJS, err := os.ReadFile(filepath.Join("cmd", "build_console", "scripts_fintech.js"))
	if err != nil {
		panic(err)
	}
	sb.Write(finJS)
	sb.WriteString("\n\n")

	entJS, err := os.ReadFile(filepath.Join("cmd", "build_console", "scripts_enterprise.js"))
	if err != nil {
		panic(err)
	}
	sb.Write(entJS)
	sb.WriteString("\n  </script>\n")
	sb.WriteString("</body>\n</html>\n")

	outPath := filepath.Join("cmd", "server", "console.html")
	if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
		panic(err)
	}

	fmt.Printf("Generated console.html successfully! Size: %d bytes\n", sb.Len())
}
