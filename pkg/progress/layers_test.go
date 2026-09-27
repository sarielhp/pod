package progress

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var packageLayers = map[string]int{
	"pod/pkg/types":    0,
	"pod/pkg/util":     0,
	"pod/pkg/progress": 0,
	"pod/pkg/format":   0,

	"pod/pkg/audio":      1,
	"pod/pkg/config":     1,
	"pod/pkg/port":       1,
	"pod/pkg/kitty":      1,
	"pod/pkg/backend":    1,
	"pod/pkg/transcribe": 1,
	"pod/pkg/detect":     1,
	"pod/pkg/gemini":     1,
	"pod/pkg/podsite":    1,

	"pod/pkg/episode": 2,

	"pod/pkg/pipeline": 3,
	"pod/pkg/player":   3,

	"pod/pkg/podcast":   4,
	"pod/pkg/adremoval": 4,

	"pod/pkg/tui": 5,
	"pod/pkg/cli": 5,
	"pod":         6,
}

func TestPackageLayering(t *testing.T) {
	pkgRoot := filepath.Clean("../..")
	fset := token.NewFileSet()

	var violations []string

	err := filepath.Walk(pkgRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "vendor" || base == ".work" || base == "tools" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, _ := filepath.Rel(pkgRoot, path)
		dir := filepath.Dir(rel)
		var importerPkg string
		if dir == "." {
			importerPkg = "pod"
		} else {
			importerPkg = "pod/" + filepath.ToSlash(dir)
		}

		importerLayer, knownImporter := packageLayers[importerPkg]
		if !knownImporter {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}

		for _, imp := range node.Imports {
			impPath := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(impPath, "pod/") {
				continue
			}
			importedLayer, knownImported := packageLayers[impPath]
			if !knownImported {
				continue
			}

			// Prohibit importing higher layers
			if importerLayer < importedLayer {
				violations = append(violations, rel+": "+importerPkg+" (layer "+string(rune('0'+importerLayer))+") imports "+impPath+" (layer "+string(rune('0'+importedLayer))+")")
			}

			// Specific anti-entanglement boundaries
			if importerPkg == "pod/pkg/podcast" && impPath == "pod/pkg/pipeline" {
				violations = append(violations, rel+": forbidden coupling: pkg/podcast must not import pkg/pipeline")
			}
			if importerPkg == "pod/pkg/adremoval" && (impPath == "pod/pkg/podcast" || impPath == "pod/pkg/backend") {
				violations = append(violations, rel+": forbidden coupling: pkg/adremoval must not import "+impPath)
			}
			if importerPkg == "pod/pkg/episode" && impPath == "pod/pkg/pipeline" {
				violations = append(violations, rel+": forbidden coupling: pkg/episode must not import pkg/pipeline")
			}
			if importerPkg == "pod/pkg/tui" && impPath == "pod/pkg/pipeline" {
				violations = append(violations, rel+": forbidden coupling: pkg/tui must not import pkg/pipeline")
			}
			if importerPkg == "pod/pkg/pipeline" && impPath == "pod/pkg/backend" {
				violations = append(violations, rel+": forbidden coupling: pkg/pipeline must not import pkg/backend")
			}
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	for _, v := range violations {
		t.Error("Layer violation: " + v)
	}
}
