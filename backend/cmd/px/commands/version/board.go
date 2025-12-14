package version

import (
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"
)

var (
	boardCmd = &cobra.Command{
		Use:   "board",
		Short: "Show version governance board summary",
		RunE:  runVersionBoard,
	}

	boardOpts = struct {
		api        string
		token      string
		tenantUUID string
		limit      int
	}{
		api:   defaultAdminAPI,
		limit: 20,
	}
)

type boardSummary struct {
	Total  int64              `json:"total"`
	ByRisk map[string]int64   `json:"riskCounts"`
	Items  []governanceReport `json:"items"`
}

func init() {
	boardCmd.Flags().StringVar(&boardOpts.api, "api", boardOpts.api, "PowerX Admin API base URL")
	boardCmd.Flags().StringVar(&boardOpts.token, "token", "", "Bearer token for Admin API")
	boardCmd.Flags().StringVar(&boardOpts.tenantUUID, "tenant-uuid", "", "Impersonate tenant via as_tenant_uuid query")
	boardCmd.Flags().IntVar(&boardOpts.limit, "limit", boardOpts.limit, "Number of reports to fetch (default 20)")
}

func runVersionBoard(cmd *cobra.Command, _ []string) error {
	client := newAPIClient(boardOpts.api, boardOpts.token)
	query := fmt.Sprintf("/internal/version/governance/board?limit=%d", boardOpts.limit)
	query = withTenantScope(query, boardOpts.tenantUUID)
	var summary boardSummary
	if err := client.do("GET", query, nil, &summary); err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Total Reports: %d\n", summary.Total)
	printRiskTable(out, summary.ByRisk)
	for _, report := range summary.Items {
		fmt.Fprintf(out, "- [%s] %s current=%s target=%s status=%s generated=%s\n",
			emptyFallback(report.RiskLevel, "info"),
			report.PluginID,
			emptyFallback(report.CurrentVersion, "n/a"),
			emptyFallback(report.RecommendedVersion, "n/a"),
			emptyFallback(report.Status, "generated"),
			report.GeneratedAt,
		)
	}
	return nil
}

func printRiskTable(out io.Writer, byRisk map[string]int64) {
	if len(byRisk) == 0 {
		fmt.Fprintln(out, "Risk counts unavailable")
		return
	}
	keys := make([]string, 0, len(byRisk))
	for k := range byRisk {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintln(out, "Risk Counts:")
	for _, key := range keys {
		fmt.Fprintf(out, "  %s\t%d\n", key, byRisk[key])
	}
}
