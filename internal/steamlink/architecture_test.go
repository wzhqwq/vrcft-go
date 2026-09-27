package steamlink

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionImportsOnlyPublicPackages(t *testing.T) {
	if cache := os.Getenv("GOCACHE"); cache == "" || !filepath.IsAbs(cache) {
		t.Fatalf("GOCACHE = %q, want an absolute repository-local cache", cache)
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", "./internal/steamlink")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list imports: %v", err)
	}

	allowed := map[string]bool{
		"github.com/wzhqwq/vrcft-go/pkg/osc":           true,
		"github.com/wzhqwq/vrcft-go/pkg/pluginapi":     true,
		"github.com/wzhqwq/vrcft-go/pkg/trackingmodel": true,
	}
	for _, imported := range strings.Fields(string(output)) {
		if strings.HasPrefix(imported, "github.com/wzhqwq/vrcft-go/") && !allowed[imported] {
			t.Fatalf("production import %q is outside the public adapter boundary", imported)
		}
	}
}
