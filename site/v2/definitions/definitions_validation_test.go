package definitions

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestRegistryNotEmpty(t *testing.T) {
	defs := v2.GetDefinitionRegistry().GetAll()
	assert.Greater(t, len(defs), 0, "registry should have at least 1 definition; ensure definitions import triggers init()")
}

func TestAllDefinitionsValidate(t *testing.T) {
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		t.Run(def.ID, func(t *testing.T) {
			err := def.Validate()
			if err != nil {
				t.Errorf("validation failed:\n%s", err)
			}
		})
	}
}

func TestUniqueIDs(t *testing.T) {
	defs := v2.GetDefinitionRegistry().GetAll()
	seen := make(map[string]bool, len(defs))
	for _, def := range defs {
		if seen[def.ID] {
			t.Errorf("duplicate site ID %q found in registry", def.ID)
		}
		seen[def.ID] = true
	}
}

func TestUniqueURLs(t *testing.T) {
	seen := make(map[string]string)
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		for _, rawURL := range def.URLs {
			normalized := normalizeURLForComparison(rawURL)
			if existing, ok := seen[normalized]; ok {
				t.Errorf("URL %q (normalized: %q) used by both %q and %q", rawURL, normalized, existing, def.ID)
			}
			seen[normalized] = def.ID
		}
	}
}

func normalizeURLForComparison(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	host := strings.ToLower(parsed.Host)
	path := strings.TrimSuffix(parsed.Path, "/")
	return parsed.Scheme + "://" + host + path
}

func TestSchemaDriversAvailable(t *testing.T) {
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		t.Run(def.ID, func(t *testing.T) {
			if def.CreateDriver != nil {
				t.Skipf("site %q uses custom CreateDriver, skipping schema driver check", def.ID)
				return
			}
			_, ok := v2.GetDriverFactoryForSchema(def.Schema.String())
			assert.True(t, ok, "no driver registered for schema %q", def.Schema)
		})
	}
}

func TestCreateDriver_Smoke(t *testing.T) {
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		if def.CreateDriver == nil {
			continue
		}
		t.Run(def.ID, func(t *testing.T) {
			config := v2.SiteConfig{
				ID:      def.ID,
				Name:    def.Name,
				Options: fakeOptionsForSchema(def.Schema),
			}
			site, err := def.CreateDriver(config, zap.NewNop())
			require.NoError(t, err)
			require.NotNil(t, site)
			assert.Equal(t, def.ID, site.ID())
		})
	}
}

func TestSiteRegistryCoversAllAvailable(t *testing.T) {
	registry := v2.NewSiteRegistry(nil)
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		t.Run(def.ID, func(t *testing.T) {
			if def.Unavailable {
				t.Skipf("site %q is marked unavailable", def.ID)
				return
			}
			_, ok := registry.Get(def.ID)
			assert.True(t, ok, "available site %q not found in SiteRegistry", def.ID)
		})
	}
}

func TestSchemaAuthConsistency(t *testing.T) {
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		t.Run(def.ID, func(t *testing.T) {
			if def.AuthMethod == "" {
				return
			}
			assert.True(t, def.AuthMethod.IsValid(),
				"AuthMethod %q is not a valid value", def.AuthMethod)
		})
	}
}

func TestDefinitionFilesHaveInit(t *testing.T) {
	defDir := findDefinitionsDir(t)
	entries, err := os.ReadDir(defDir)
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		t.Run(name, func(t *testing.T) {
			filePath := filepath.Join(defDir, name)
			node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
			require.NoError(t, err)

			hasInit := false
			hasRegisterCall := false

			ast.Inspect(node, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok {
					return true
				}
				if fn.Name.Name == "init" && fn.Recv == nil {
					hasInit = true
					ast.Inspect(fn.Body, func(inner ast.Node) bool {
						call, ok := inner.(*ast.CallExpr)
						if !ok {
							return true
						}
						if containsRegisterCall(call) {
							hasRegisterCall = true
						}
						return true
					})
				}
				return true
			})

			assert.True(t, hasInit,
				"file %q must have an init() function that registers the site definition", name)
			if hasInit {
				assert.True(t, hasRegisterCall,
					"init() in %q must call v2.RegisterSiteDefinition() or RegisterSiteDefinition()", name)
			}
		})
	}
}

func containsRegisterCall(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name == "RegisterSiteDefinition"
	case *ast.SelectorExpr:
		return fn.Sel.Name == "RegisterSiteDefinition"
	}
	return false
}

func TestDefinitionFileContainsSiteID(t *testing.T) {
	defDir := findDefinitionsDir(t)
	entries, err := os.ReadDir(defDir)
	require.NoError(t, err)

	registeredIDs := make(map[string]bool)
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		registeredIDs[def.ID] = true
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		t.Run(name, func(t *testing.T) {
			filePath := filepath.Join(defDir, name)
			src, err := os.ReadFile(filePath)
			require.NoError(t, err)

			node, err := parser.ParseFile(fset, filePath, src, parser.AllErrors)
			require.NoError(t, err)

			foundIDs := extractSiteIDs(node)
			for _, id := range foundIDs {
				if registeredIDs[id] {
					return
				}
			}

			baseName := strings.TrimSuffix(name, ".go")
			if registeredIDs[baseName] {
				return
			}

			t.Logf("warning: file %q does not obviously correspond to a registered site ID (found IDs in AST: %v)", name, foundIDs)
		})
	}
}

func extractSiteIDs(node *ast.File) []string {
	var ids []string
	ast.Inspect(node, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok || ident.Name != "ID" {
			return true
		}
		lit, ok := kv.Value.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		id := strings.Trim(lit.Value, "\"")
		ids = append(ids, id)
		return true
	})
	return ids
}

func findDefinitionsDir(t *testing.T) string {
	t.Helper()
	candidates := []string{
		".",
		"site/v2/definitions",
		"../../../site/v2/definitions",
	}

	wd, _ := os.Getwd()
	if strings.HasSuffix(wd, "site/v2/definitions") {
		return wd
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if strings.HasSuffix(abs, "site/v2/definitions") {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
		full := filepath.Join(abs, "site/v2/definitions")
		if _, err := os.Stat(full); err == nil {
			return full
		}
	}

	t.Fatal("could not find site/v2/definitions directory")
	return ""
}

func fakeOptionsForSchema(schema v2.Schema) json.RawMessage {
	switch schema {
	case v2.SchemaRousi:
		return json.RawMessage(`{"passkey":"FAKE_TEST_PASSKEY_1234"}`)
	default:
		return json.RawMessage(`{}`)
	}
}

// fixtureSuiteLegacy lists sites that existed before the FixtureSuite requirement.
// New sites MUST NOT be added here — register a FixtureSuite instead.
var fixtureSuiteLegacy = map[string]bool{}

func TestAllSites_FixtureCoverage(t *testing.T) {
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		t.Run(def.ID, func(t *testing.T) {
			if def.Unavailable {
				t.Skipf("site %q is marked unavailable", def.ID)
				return
			}

			suite, registered := fixtureRegistry[def.ID]
			if !registered {
				if fixtureSuiteLegacy[def.ID] {
					t.Skipf("legacy site %q — FixtureSuite not yet added", def.ID)
					return
				}
				t.Fatalf("site %q has no registered FixtureSuite. "+
					"Add RegisterFixtureSuite() in %s_fixture_test.go — see docs/development.md",
					def.ID, def.ID)
			}

			if suite.Search == nil {
				t.Errorf("site %q FixtureSuite.Search is nil", def.ID)
			} else {
				t.Run("Search", suite.Search)
			}

			if suite.Detail == nil {
				t.Errorf("site %q FixtureSuite.Detail is nil", def.ID)
			} else {
				t.Run("Detail", suite.Detail)
			}

			if suite.UserInfo == nil {
				t.Errorf("site %q FixtureSuite.UserInfo is nil", def.ID)
			} else {
				t.Run("UserInfo", suite.UserInfo)
			}
		})
	}
}
