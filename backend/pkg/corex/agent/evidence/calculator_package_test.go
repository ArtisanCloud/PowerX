package evidence

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalculatorImplementationPackageContainsActualSources(t *testing.T) {
	raw, err := CalculatorPackageBytes()
	require.NoError(t, err)
	var bundle struct {
		Files map[string][]byte `json:"files"`
	}
	require.NoError(t, json.Unmarshal(raw, &bundle))
	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, name := range paths {
		if strings.HasSuffix(name, "_test.go") {
			require.NotContains(t, bundle.Files, name)
			continue
		}
		source, err := os.ReadFile(name)
		require.NoError(t, err)
		require.Equal(t, source, bundle.Files[name])
	}
	source, err := os.ReadFile("../../../utils/json.go")
	require.NoError(t, err)
	require.Equal(t, source, bundle.Files["utils/json.go"])
	descriptor, err := NewToolService(ToolServiceOptions{}).CalculatorDependency("calculator")
	require.NoError(t, err)
	require.Equal(t, toolDigest(raw), descriptor.Binding.ImplementationSHA256)
}

func TestCalculatorDescriptorIsStableAcrossActualBinaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "probe.go")
	code := `package main
import("encoding/json";"os";"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence")
func main(){d,e:=evidence.NewToolService(evidence.ToolServiceOptions{}).CalculatorDependency("calculator");if e!=nil{panic(e)};json.NewEncoder(os.Stdout).Encode(d)}
`
	require.NoError(t, os.WriteFile(path, []byte(code), 0600))
	command := exec.CommandContext(ctx, "go", "run", path)
	command.Dir = "../../../.."
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	var actual ToolDependency
	require.NoError(t, DecodeToolJSON(output, &actual))
	expected, err := NewToolService(ToolServiceOptions{}).CalculatorDependency("calculator")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
