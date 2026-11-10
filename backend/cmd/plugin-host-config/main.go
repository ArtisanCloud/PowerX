// plugin-host-config 默认只读预览旧 Core 注入的 License cache 配置。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
)

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("plugin-host-config", flag.ContinueOnError)
	flags.SetOutput(out)
	registry := flags.String("registry", "", "Core registry.json path")
	plugin := flags.String("plugin", "", "plugin ID")
	version := flags.String("version", "", "installed version; default current")
	apply := flags.Bool("apply", false, "apply a reviewed cleanup during a stopped-Core window")
	confirm := flags.Bool("confirm-generated", false, "operator confirms this candidate was produced by old Core injection")
	stopped := flags.Bool("core-stopped", false, "operator confirms Core is stopped; does not stop Core")
	digest := flags.String("expect", "", "plan_digest from the reviewed preview")
	backup := flags.String("backup-dir", "", "new private backup directory outside the plugin package")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *registry == "" || *plugin == "" || flags.NArg() != 0 {
		return fmt.Errorf("require -registry and -plugin")
	}
	var report manager.LegacyLicenseCachePreview
	var err error
	if *apply {
		report, err = manager.RepairLegacyLicenseCache(context.Background(), *registry, *plugin, *version, *digest, *backup, *confirm, *stopped)
	} else {
		report, err = manager.PreviewLegacyLicenseCache(context.Background(), *registry, *plugin, *version)
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
