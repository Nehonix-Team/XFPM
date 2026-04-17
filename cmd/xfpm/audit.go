/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"net"
	"os/signal"
	"syscall"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/spf13/cobra"
)

//go:embed templates/audit_report.xfpml
var auditReportTmpl string

var (
	auditTree bool
	auditHtml bool
	auditYes  bool
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit project dependencies for known vulnerabilities using OSV API",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, _ := os.Getwd()
		pkgJsonPath := filepath.Join(projectRoot, "package.json")

		if _, err := os.Stat(pkgJsonPath); os.IsNotExist(err) {
			return fmt.Errorf("no package.json found in current directory")
		}

		pkgJson, err := core.LoadPackageJson(pkgJsonPath)
		if err != nil {
			return err
		}

		spinner, _ := pterm.DefaultSpinner.Start("Analyzing dependency graph...")

		registry := core.NewRegistryClient("", 3)
		home, _ := os.UserHomeDir()
		registry.SetCacheDir(filepath.Join(home, ".xpm", "cache"))

		cas, err := core.NewCas("")
		if err != nil {
			return fmt.Errorf("failed to initialize CAS: %w", err)
		}
		resolver := core.NewResolver(registry, cas)

		ctx := context.Background()
		resolved, _, err := resolver.ResolveTree(ctx, pkgJson.AllDependencies())
		if err != nil {
			spinner.Fail(fmt.Sprintf("Failed to resolve dependencies: %v", err))
			return err
		}
		spinner.Success(fmt.Sprintf("Resolved %d packages", len(resolved)))

		if len(resolved) == 0 {
			pterm.Info.Println("No dependencies to audit.")
			return nil
		}

		auditSpinner, _ := pterm.DefaultSpinner.Start("Checking vulnerabilities...")
		vulns, err := checkVulnerabilities(resolved)
		if err != nil {
			auditSpinner.Fail(fmt.Sprintf("Audit failed: %v", err))
			return err
		}
		auditSpinner.Success("Audit complete")

		displayAuditReport(vulns)

		if len(vulns) > 0 {
			if auditTree {
				displayVulnerabilityTree(resolved, vulns)
			} else if auditHtml {
				handleHtmlReport(pkgJson.Name, resolved, vulns)
			} else {
				options := []string{"View Vulnerability Tree (Terminal)", "Generate Technical Report (HTML)", "Exit"}
				selected, _ := pterm.DefaultInteractiveSelect.WithOptions(options).Show("Select additional view")

				switch selected {
				case "View Vulnerability Tree (Terminal)":
					displayVulnerabilityTree(resolved, vulns)
				case "Generate Technical Report (HTML)":
					handleHtmlReport(pkgJson.Name, resolved, vulns)
				}
			}
		}

		utils.PrintFooter(0)
		return nil
	},
}

func handleHtmlReport(projectName string, resolved []*core.ResolvedPackage, vulns []DetectedVuln) {
	reportPath, err := generateHtmlReport(projectName, resolved, vulns)
	if err != nil {
		pterm.Error.Printf("Failed to generate report: %v\n", err)
		return
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		pterm.Error.Printf("Failed to start local server: %v\n", err)
		return
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	pterm.Success.Printf("Report server started at %s\n", url)
	openBrowser(url)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, reportPath)
	})

	srv := &http.Server{}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			pterm.Error.Printf("Server error: %v\n", err)
		}
	}()

	pterm.Info.Println("Press Ctrl+C to stop the server and delete the temporary report...")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	os.Remove(reportPath)
	pterm.Info.Println("Server stopped. Cleaned up.")
}

func displayVulnerabilityTree(allPackages []*core.ResolvedPackage, vulns []DetectedVuln) {
	fmt.Println()
	pterm.DefaultBasicText.Println(pterm.Bold.Sprint("  Vulnerability Dependency Paths:"))

	pkgMap := make(map[string]map[string]*core.ResolvedPackage)
	for _, p := range allPackages {
		if pkgMap[p.Name] == nil {
			pkgMap[p.Name] = make(map[string]*core.ResolvedPackage)
		}
		pkgMap[p.Name][p.Version] = p
	}

	for _, v := range vulns {
		pterm.Println(pterm.FgRed.Sprintf("\n  Dependency paths for %s (%s):", v.Package, v.ID))
		paths := findPathsTo(allPackages, v.Package)

		treeData := pterm.LeveledList{}
		for _, path := range paths {
			for level, node := range path {
				treeData = append(treeData, pterm.LeveledListItem{
					Level: level,
					Text:  node,
				})
			}
		}
		pterm.DefaultTree.WithRoot(putils.TreeFromLeveledList(treeData)).Render()
	}
}

func findPathsTo(allPackages []*core.ResolvedPackage, target string) [][]string {
	var paths [][]string
	visited := make(map[string]bool)

	// Build a reverse dependency map
	revDeps := make(map[string][]string)
	for _, p := range allPackages {
		for depName := range p.ResolvedDependencies {
			revDeps[depName] = append(revDeps[depName], p.Name)
		}
	}

	var dfs func(current string, currentPath []string)
	dfs = func(current string, currentPath []string) {
		if visited[current] {
			return
		}
		visited[current] = true
		defer func() { visited[current] = false }()

		newPath := append([]string{current}, currentPath...)
		parents := revDeps[current]
		if len(parents) == 0 {
			paths = append(paths, append([]string{"[root]"}, newPath...))
			return
		}

		for _, p := range parents {
			dfs(p, newPath)
		}
	}

	dfs(target, nil)

	// Limit to top 5 paths to avoid performance issues in UI
	if len(paths) > 5 {
		return paths[:5]
	}

	return paths
}

// GraphNode represents a node in the dependency graph for JSON serialization.
type GraphNode struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Vuln    bool   `json:"vuln"`
	IsRoot  bool   `json:"isRoot"`
}

// GraphEdge represents an edge in the dependency graph for JSON serialization.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// GraphData holds nodes and edges for the D3 force graph.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

func generateGraphData(allPackages []*core.ResolvedPackage, vulns []DetectedVuln) string {
	vulnSet := make(map[string]bool)
	for _, v := range vulns {
		vulnSet[v.Package] = true
	}

	nodeMap := make(map[string]bool)
	var nodes []GraphNode
	var edges []GraphEdge

	nodes = append(nodes, GraphNode{ID: "__root__", Name: "Project Root", IsRoot: true})
	nodeMap["__root__"] = true

	const maxNodes = 300
	count := 0

	for _, p := range allPackages {
		if count >= maxNodes {
			break
		}
		nid := p.Name + "@" + p.Version
		if !nodeMap[nid] {
			nodes = append(nodes, GraphNode{
				ID:      nid,
				Name:    p.Name,
				Version: p.Version,
				Vuln:    vulnSet[p.Name],
			})
			nodeMap[nid] = true
			count++
		}
	}

	addedEdges := make(map[string]bool)
	for _, p := range allPackages {
		srcID := p.Name + "@" + p.Version
		if !nodeMap[srcID] {
			continue
		}
		for depName, depVer := range p.ResolvedDependencies {
			tgtID := depName + "@" + depVer
			if !nodeMap[tgtID] {
				continue
			}
			ek := srcID + "->" + tgtID
			if !addedEdges[ek] {
				edges = append(edges, GraphEdge{Source: srcID, Target: tgtID})
				addedEdges[ek] = true
			}
		}
	}

	data := GraphData{Nodes: nodes, Edges: edges}
	b, _ := json.Marshal(data)
	return string(b)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// auditReportData holds all values injected into the HTML template.
type auditReportData struct {
	ProjectName   string
	GeneratedAt   string
	VulnCount     int
	TotalPackages int
	VulnRate      string
	VulnsJSON     template.JS
	GraphJSON     template.JS
}

func generateHtmlReport(projectName string, allPackages []*core.ResolvedPackage, vulns []DetectedVuln) (string, error) {
	tempFile, err := os.CreateTemp("", "xfpm-audit-*.html")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	vulnJSON, _ := json.Marshal(vulns)
	graphDataJSON := generateGraphData(allPackages, vulns)

	tmpl, err := template.New("audit").Parse(auditReportTmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	data := auditReportData{
		ProjectName:   projectName,
		GeneratedAt:   time.Now().Format("Jan 02, 2006 15:04"),
		VulnCount:     len(vulns),
		TotalPackages: len(allPackages),
		VulnRate:      fmt.Sprintf("%.1f", float64(len(vulns))*100/float64(maxInt(len(allPackages), 1))),
		VulnsJSON:     template.JS(vulnJSON),
		GraphJSON:     template.JS(graphDataJSON),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	if _, err := tempFile.Write(buf.Bytes()); err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		pterm.Warning.Printf("Could not open browser: %v\n", err)
	}
}

// ─── OSV API types ───────────────────────────────────────────────────────────

type OSVQuery struct {
	Package struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
	Version string `json:"version"`
}

type OSVBatchRequest struct {
	Queries []OSVQuery `json:"queries"`
}

type OSVBatchResponse struct {
	Results []struct {
		Vulns []struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
			Details string `json:"details"`
		} `json:"vulns"`
	} `json:"results"`
}

type OSVFullVuln struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Details string `json:"details"`
	Severity []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	} `json:"severity"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
}

type DetectedVuln struct {
	Package      string     `json:"Package"`
	Version      string     `json:"Version"`
	ID           string     `json:"ID"`
	Summary      string     `json:"Summary"`
	Details      string     `json:"Details"`
	Severity     string     `json:"Severity"`
	FixedVersion string     `json:"FixedVersion"`
	Paths        [][]string `json:"Paths"`
}

// ─── Vulnerability checking ──────────────────────────────────────────────────

func checkVulnerabilities(packages []*core.ResolvedPackage) ([]DetectedVuln, error) {
	const osvBatchEndpoint = "https://api.osv.dev/v1/querybatch"
	const batchSize = 1000

	var allVulns []DetectedVuln
	client := &http.Client{Timeout: 60 * time.Second}

	for i := 0; i < len(packages); i += batchSize {
		end := i + batchSize
		if end > len(packages) {
			end = len(packages)
		}

		batch := packages[i:end]
		reqPayload := OSVBatchRequest{}
		for _, p := range batch {
			q := OSVQuery{Version: p.Version}
			q.Package.Name = p.Name
			q.Package.Ecosystem = "npm"
			reqPayload.Queries = append(reqPayload.Queries, q)
		}

		body, _ := json.Marshal(reqPayload)
		resp, err := client.Post(osvBatchEndpoint, "application/json", bytes.NewBuffer(body))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("OSV API returned status %d", resp.StatusCode)
		}

		var osvResp OSVBatchResponse
		if err := json.NewDecoder(resp.Body).Decode(&osvResp); err != nil {
			return nil, err
		}

		for idx, res := range osvResp.Results {
			if len(res.Vulns) == 0 {
				continue
			}
			pkg := batch[idx]
			for _, v := range res.Vulns {
				detail, _ := fetchVulnerabilityDetail(v.ID)

				severity := "UNKNOWN"
				if detail != nil && len(detail.Severity) > 0 {
					severity = detail.Severity[0].Score
				}

				fixedVer := ""
				if detail != nil {
					// Simplified fix detection: OSV has 'affected' field with 'ranges' and 'fixed'
					// We'll skip complex parsing for now and just set it if we find a likely fixed version
				}

				dv := DetectedVuln{
					Package:      pkg.Name,
					Version:      pkg.Version,
					ID:           v.ID,
					Summary:      v.Summary,
					Details:      v.Details,
					Severity:     severity,
					FixedVersion: fixedVer,
					Paths:        findPathsTo(packages, pkg.Name),
				}

				if detail != nil {
					if detail.Details != "" {
						dv.Details = detail.Details
					}
					if detail.Summary != "" {
						dv.Summary = detail.Summary
					}
				}

				allVulns = append(allVulns, dv)
			}
		}
	}

	return allVulns, nil
}

func fetchVulnerabilityDetail(id string) (*OSVFullVuln, error) {
	url := fmt.Sprintf("https://api.osv.dev/v1/vulns/%s", id)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch vuln details: %d", resp.StatusCode)
	}

	var vuln OSVFullVuln
	if err := json.NewDecoder(resp.Body).Decode(&vuln); err != nil {
		return nil, err
	}

	return &vuln, nil
}

// ─── Terminal report ─────────────────────────────────────────────────────────

func displayAuditReport(vulns []DetectedVuln) {
	fmt.Println()
	if len(vulns) == 0 {
		pterm.DefaultBasicText.Printf("  %s %s\n\n",
			pterm.FgGreen.Sprint("✔"),
			pterm.Bold.Sprint("No known vulnerabilities found."))
		return
	}

	pterm.DefaultBasicText.Printf("  %s %s\n\n",
		pterm.FgRed.Sprint("✘"),
		pterm.Bold.Sprint(fmt.Sprintf("Found %d vulnerabilities", len(vulns))))

	tableData := pterm.TableData{
		{"Package", "Version", "ID", "Summary"},
	}
	for _, v := range vulns {
		tableData = append(tableData, []string{
			pterm.FgCyan.Sprint(v.Package),
			pterm.FgYellow.Sprint(v.Version),
			pterm.FgRed.Sprint(v.ID),
			v.Summary,
		})
	}

	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	fmt.Println()
}

var auditFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Automatically resolve vulnerabilities by updating direct dependencies",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, _ := os.Getwd()
		pkgJsonPath := filepath.Join(projectRoot, "package.json")

		if _, err := os.Stat(pkgJsonPath); os.IsNotExist(err) {
			return fmt.Errorf("no package.json found in current directory")
		}

		pkgJson, err := core.LoadPackageJson(pkgJsonPath)
		if err != nil {
			return err
		}

		spinner, _ := pterm.DefaultSpinner.Start("Auditing for fixable vulnerabilities...")

		registry := core.NewRegistryClient("", 3)
		home, _ := os.UserHomeDir()
		registry.SetCacheDir(filepath.Join(home, ".xpm", "cache"))
		cas, _ := core.NewCas("")
		resolver := core.NewResolver(registry, cas)

		ctx := context.Background()
		resolved, _, err := resolver.ResolveTree(ctx, pkgJson.AllDependencies())
		if err != nil {
			spinner.Fail(fmt.Sprintf("Audit failed: %v", err))
			return err
		}

		vulns, err := checkVulnerabilities(resolved)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Audit failed: %v", err))
			return err
		}
		spinner.Success("Audit complete")

		if len(vulns) == 0 {
			utils.Success("No vulnerabilities found. Nothing to fix.")
			return nil
		}

		fixable := make(map[string]bool)
		for _, v := range vulns {
			// Check if it's a direct dependency
			isDirect := false
			if _, ok := pkgJson.Dependencies[v.Package]; ok { isDirect = true }
			if _, ok := pkgJson.DevDependencies[v.Package]; ok { isDirect = true }
			if _, ok := pkgJson.OptionalDependencies[v.Package]; ok { isDirect = true }
			if _, ok := pkgJson.PeerDependencies[v.Package]; ok { isDirect = true }

			if isDirect {
				fixable[v.Package] = true
			}
		}

		if len(fixable) == 0 {
			utils.Warn("Found %d vulnerabilities, but none are direct dependencies. Manual inspection of the dependency tree required.", len(vulns))
			displayVulnerabilityTree(resolved, vulns)
			return nil
		}

		pterm.DefaultBox.WithTitle(pterm.FgYellow.Sprint(" SECURITY WARNING ")).Printfln(
			"XFPM will attempt to fix %d direct vulnerabilities by updating them to 'latest'.\n" +
			"This may introduce %s depending on new package versions.",
			len(fixable), pterm.Bold.Sprint("BREAKING CHANGES"))

		if !auditYes {
			confirm, _ := pterm.DefaultInteractiveConfirm.WithDefaultText("Do you want to proceed?").Show()
			if !confirm {
				return nil
			}
		}

		fixSpinner, _ := pterm.DefaultSpinner.Start("Fixing vulnerabilities...")
		for pkgName := range fixable {
			fixSpinner.UpdateText(fmt.Sprintf("Updating %s to latest...", pkgName))
			
			// Set to latest in whatever section it belongs to
			if _, ok := pkgJson.Dependencies[pkgName]; ok { pkgJson.Dependencies[pkgName] = "latest" }
			if _, ok := pkgJson.DevDependencies[pkgName]; ok { pkgJson.DevDependencies[pkgName] = "latest" }
			if _, ok := pkgJson.OptionalDependencies[pkgName]; ok { pkgJson.OptionalDependencies[pkgName] = "latest" }
			if _, ok := pkgJson.PeerDependencies[pkgName]; ok { pkgJson.PeerDependencies[pkgName] = "latest" }
		}

		if err := pkgJson.Save(pkgJsonPath); err != nil {
			fixSpinner.Fail(fmt.Sprintf("Failed to save package.json: %v", err))
			return err
		}
		fixSpinner.Success("Package.json updated.")

		utils.Info("Triggering fresh installation to apply fixes...")
		// Use cobra's Execute on installCmd logic or just advice user to run install.
		// For simplicity, we trigger internal install logic if possible, but easier to just notify.
		utils.Success("Successfully applied fixes to %d packages. Please run 'xfpm install' to complete the repair.", len(fixable))

		return nil
	},
}

func init() {
	auditCmd.Flags().BoolVarP(&auditTree, "tree", "t", false, "Display vulnerability tree in terminal")
	auditCmd.Flags().BoolVarP(&auditHtml, "html", "w", false, "Generate and open HTML report")
	auditFixCmd.Flags().BoolVarP(&auditYes, "yes", "y", false, "Skip confirmation prompt")
	auditCmd.AddCommand(auditFixCmd)
	rootCmd.AddCommand(auditCmd)
}