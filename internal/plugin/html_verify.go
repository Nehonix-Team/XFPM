package plugin

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
)

//go:embed templates/plugin_verify.xfpml
var pluginVerifyTmpl string

var (
	clients           []chan string
	clientsMu         sync.Mutex
	plugins           []WebPlugin
	mu                sync.Mutex
	broadcast         func(string)
	masterPermissions map[string]struct {
		Name        string `json:"name"`
		Action      string `json:"action"`
		Description string `json:"description"`
	}
)

func fetchMasterPermissions() {
	if masterPermissions != nil {
		return
	}
	resp, err := http.Get("https://raw.githubusercontent.com/Nehonix-Team/XyPriss/master/.data/base/permissions.json")
	if err == nil {
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&masterPermissions)
	}
}

func buildWebPlugins(projectRoot string, pending []PendingReq, config map[string]interface{}, isReview bool) []WebPlugin {
	fetchMasterPermissions()
	var wps []WebPlugin
	for _, p := range pending {
		status := p.Status
		pkgDir_ := paths.NodeModulesPkgDir(projectRoot, p.Name)
		if _, err := os.Stat(pkgDir_); os.IsNotExist(err) && status != "authorized" {
			status = "NOT_INSTALLED"
		}

		wp := WebPlugin{
			Name:       p.Name,
			Version:    p.Version,
			Identity:   p.Identity,
			Privileges: p.Privileges,
			Status:     status,
		}

		currentPerms := make(map[string]bool)
		if internal, ok := config["$internal"].(map[string]interface{}); ok {
			if pluginCfg, ok := internal[p.Name].(map[string]interface{}); ok {
				if permCfg, ok := pluginCfg["permissions"].(map[string]interface{}); ok {
					if allowed, ok := permCfg["allowedHooks"].([]interface{}); ok {
						for _, a := range allowed {
							if s, ok := a.(string); ok {
								currentPerms[s] = true
							}
						}
					} else if allowed, ok := permCfg["allowedHooks"].([]string); ok {
						for _, a := range allowed {
							currentPerms[a] = true
						}
					}
				}
			}
		}

		if p.Privileges != "" && p.Privileges != "none" {
			for _, id := range strings.Split(p.Privileges, ",") {
				id = strings.TrimSpace(id)
				detail := masterPermissions[id]
				wp.RequestedPrivileges = append(wp.RequestedPrivileges, WebPerm{
					ID:          id,
					Name:        detail.Name,
					Action:      detail.Action,
					Description: detail.Description,
					Approved:    currentPerms[id] || (p.Status != "authorized" && !isReview),
				})
			}
		}
		wps = append(wps, wp)
	}
	return wps
}

func renderTerminalTable(pList []WebPlugin) string {
	items := [][]string{
		{"Plugin", "Version", "Status", "Developer ID"},
	}

	for _, p := range pList {
		status := p.Status
		var statusStyled string
		switch status {
		case "authorized":
			statusStyled = pterm.Green("VERIFIED")
		case "NOT_INSTALLED":
			statusStyled = pterm.Magenta("NOT INSTALLED")
		default:
			statusStyled = pterm.Red("UNTRUSTED")
		}

		idStyled := p.Identity
		if idStyled == "" {
			idStyled = "-"
		}

		items = append(items, []string{
			p.Name,
			p.Version,
			statusStyled,
			idStyled,
		})
	}

	table, _ := pterm.DefaultTable.WithHasHeader().WithData(items).Srender()
	return pterm.DefaultBox.WithTitle(utils.AccentColor.Sprint("PROJECT PLUGINS")).Sprint(table)
}

func HandleHtmlVerify(projectRoot string, pending []PendingReq, config map[string]interface{}, configPath string, isReview bool) {
	utils.Matrix("Initializing verification metadata...")
	mu.Lock()
	plugins = buildWebPlugins(projectRoot, pending, config, isReview)
	mu.Unlock()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		pterm.Error.Printf("Failed to start local server: %v\n", err)
		return
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	pterm.Success.Printf("Verification dashboard started at %s\n", url)
	pterm.Info.Println("Press Ctrl+C to cancel and return to terminal...")
	utils.OpenBrowser(url)

	area, _ := pterm.DefaultArea.Start()

	broadcast = func(msg string) {
		clientsMu.Lock()
		for _, ch := range clients {
			select {
			case ch <- msg:
			default:
			}
		}
		clientsMu.Unlock()

		mu.Lock()
		currentPlugins := make([]WebPlugin, len(plugins))
		copy(currentPlugins, plugins)
		mu.Unlock()
		area.Update(renderTerminalTable(currentPlugins))
	}

	area.Update(renderTerminalTable(plugins))

	done := make(chan bool)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		tmpl, _ := template.New("verify").Parse(pluginVerifyTmpl)
		tmpl.Execute(w, struct {
			Plugins  []WebPlugin
			IsReview bool
		}{
			Plugins:  plugins,
			IsReview: isReview,
		})
	})

	http.HandleFunc("/restart", func(w http.ResponseWriter, r *http.Request) {
		pterm.Info.Print("\nPreparing restart...\n")
		
		// 1. Re-scan project state
		content, _ := os.ReadFile(configPath)
		var freshConfig map[string]interface{}
		json.Unmarshal(content, &freshConfig)
		pendingPrompt, _ := ScanProjectPlugins(projectRoot, freshConfig, func(percent int, msg string) {
			b, _ := json.Marshal(map[string]interface{}{
				"event":   "progress",
				"percent": percent,
				"msg":     msg,
			})
			broadcast("event: progress\ndata: " + string(b))
		})

		// 2. Stop Area FIRST, then clear to ensure no buffer restoration
		area.Stop()
		utils.ClearTerminal()

		// 3. Spawn a NEW session in parallel
		// We'll use a channel to get the new port
		mu.Lock()
		plugins = buildWebPlugins(projectRoot, pendingPrompt, freshConfig, isReview)
		mu.Unlock()
		
		pterm.Success.Print("\nDeep discovery complete. Dashboard state synchronized.\n")
		
		area, _ = pterm.DefaultArea.Start()
		area.Update(renderTerminalTable(plugins))
		
		broadcast("event: reload\ndata: ok")
		w.WriteHeader(200)
	})

	http.HandleFunc("/refresh", func(w http.ResponseWriter, r *http.Request) {
		pterm.Info.Println("Refreshing plugin metadata...")
		
		// Re-read config for fresh permissions
		content, _ := os.ReadFile(configPath)
		var freshConfig map[string]interface{}
		json.Unmarshal(content, &freshConfig)

		// Re-build pending requests based on current plugins to preserve their identity/version
		mu.Lock()
		var pReqs []PendingReq
		for _, p := range plugins {
			pReqs = append(pReqs, PendingReq{
				Name:       p.Name,
				Version:    p.Version,
				Identity:   p.Identity,
				Privileges: p.Privileges,
				Status:     p.Status,
			})
		}
		plugins = buildWebPlugins(projectRoot, pReqs, freshConfig, isReview)
		b, _ := json.Marshal(plugins)
		mu.Unlock()

		broadcast(string(b))
		w.WriteHeader(200)
		pterm.Success.Print("\nMetadata refresh complete.\n")
	})

	http.HandleFunc("/apply", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Data  map[string]string `json:"data"`
			Close bool              `json:"close"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		content, err := os.ReadFile(configPath)
		if err != nil {
			http.Error(w, "Could not read config", 500)
			return
		}
		var config map[string]interface{}
		if err := json.Unmarshal(content, &config); err != nil {
			http.Error(w, "Could not parse config", 500)
			return
		}

		internalRaw, ok := config["$internal"]
		if !ok {
			internalRaw = make(map[string]interface{})
		}
		internal, ok := internalRaw.(map[string]interface{})
		if !ok {
			internal = make(map[string]interface{})
		}

		mu.Lock()
		currentPlugins := make([]WebPlugin, len(plugins))
		copy(currentPlugins, plugins)
		mu.Unlock()

		authorizedCount := 0
		revokedCount := 0

		for _, p := range currentPlugins {
			val, exists := req.Data["trust-"+p.Name]
			isTrusted := exists && val == "on"

			if !isTrusted {
				if exists && isReview && p.Status == "authorized" {
					if err := RevokeTrust(projectRoot, p.Name, false); err != nil {
						utils.Error("Failed to revoke trust for %s: %v", p.Name, err)
					}
					delete(internal, p.Name)
					mu.Lock()
					for i, wp := range plugins {
						if wp.Name == p.Name {
							plugins[i].Status = "pending"
						}
					}
					mu.Unlock()
					revokedCount++
					pterm.Warning.Printf("Plugin %s trust revoked via UI\n", p.Name)
				}
				continue
			}

			pluginCfgRaw, ok := internal[p.Name]
			if !ok {
				pluginCfgRaw = make(map[string]interface{})
			}
			pluginCfg, ok := pluginCfgRaw.(map[string]interface{})
			if !ok {
				pluginCfg = make(map[string]interface{})
			}

			if p.Identity != "" && p.Identity != "-" {
				mu.Lock()
				for i, wp := range plugins {
					if wp.Name == p.Name {
						plugins[i].Status = "authorized"
					}
				}
				mu.Unlock()

				sigCfgRaw, ok := pluginCfg["signature"]
				if !ok {
					sigCfgRaw = make(map[string]interface{})
				}
				sigCfg, ok := sigCfgRaw.(map[string]interface{})
				if !ok {
					sigCfg = make(map[string]interface{})
				}
				sigCfg["author_key"] = p.Identity
				pluginCfg["signature"] = sigCfg
			}

			var approved []string
			if p.Privileges != "" && p.Privileges != "none" {
				for _, id := range strings.Split(p.Privileges, ",") {
					id = strings.TrimSpace(id)
					perm := "perm-" + p.Name + "-" + id
					if req.Data[perm] == "on" {
						approved = append(approved, id)
					}
				}
			}

			permCfgRaw, ok := pluginCfg["permissions"]
			if !ok {
				permCfgRaw = make(map[string]interface{})
			}
			permCfg, ok := permCfgRaw.(map[string]interface{})
			if !ok {
				permCfg = make(map[string]interface{})
			}
			permCfg["allowedHooks"] = approved
			pluginCfg["permissions"] = permCfg

			internal[p.Name] = pluginCfg
			authorizedCount++
			pterm.Success.Printf("Plugin %s authorized with %d privileges\n", p.Name, len(approved))
		}
		config["$internal"] = internal

		if out, err := json.MarshalIndent(config, "", "    "); err == nil {
			os.WriteFile(configPath, out, 0644)
		}
		
		w.WriteHeader(200)

		mu.Lock()
		b, _ := json.Marshal(plugins)
		mu.Unlock()
		broadcast(string(b))

		if req.Close {
			done <- true
		}
	})

	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		ch := make(chan string, 1)
		clientsMu.Lock()
		clients = append(clients, ch)
		clientsMu.Unlock()

		defer func() {
			clientsMu.Lock()
			for i, c := range clients {
				if c == ch {
					clients = append(clients[:i], clients[i+1:]...)
					break
				}
			}
			remaining := len(clients)
			clientsMu.Unlock()

			if remaining == 0 {
				go func() {
					time.Sleep(3 * time.Second)
					clientsMu.Lock()
					if len(clients) == 0 {
						done <- true
					}
					clientsMu.Unlock()
				}()
			}
		}()

		for {
			select {
			case msg := <-ch:
				if strings.HasPrefix(msg, "event:") {
					fmt.Fprintf(w, "%s\n\n", msg)
				} else {
					fmt.Fprintf(w, "data: %s\n\n", msg)
				}
				w.(http.Flusher).Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	http.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(400)
			return
		}
		pkgName := r.FormValue("package")
		if pkgName == "" {
			w.WriteHeader(400)
			return
		}
		noPending := r.FormValue("noPending") == "true"
		err = RevokeTrust(projectRoot, pkgName, noPending)
		if err != nil {
			utils.Error("Failed to remotely revoke %s: %v", pkgName, err)
			w.WriteHeader(500)
			return
		}
		mu.Lock()
		for i, wp := range plugins {
			if wp.Name == pkgName {
				if noPending {
					plugins[i].Status = "NOT_INSTALLED"
				} else {
					plugins[i].Status = "pending"
				}
			}
		}
		mu.Unlock()
		w.WriteHeader(200)
		pterm.Warning.Printf("Plugin %s revoked (noPending: %v)\n", pkgName, noPending)
		mu.Lock()
		b, _ := json.Marshal(plugins)
		mu.Unlock()
		broadcast(string(b))
	})

	http.HandleFunc("/revoke-batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var req struct {
			Packages  []string `json:"packages"`
			NoPending bool     `json:"noPending"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		failed := false
		for _, pkgName := range req.Packages {
			err := RevokeTrust(projectRoot, pkgName, req.NoPending)
			if err != nil {
				utils.Error("Failed to remotely revoke %s: %v", pkgName, err)
				failed = true
				continue
			}
			for i, wp := range plugins {
				if wp.Name == pkgName {
					if req.NoPending {
						plugins[i].Status = "NOT_INSTALLED"
					} else {
						plugins[i].Status = "pending"
					}
				}
			}
			pterm.Warning.Printf("Plugin %s revoked in batch\n", pkgName)
		}
		mu.Unlock()
		if failed {
			w.WriteHeader(207)
		} else {
			w.WriteHeader(200)
		}
		mu.Lock()
		b, _ := json.Marshal(plugins)
		mu.Unlock()
		broadcast(string(b))
	})

	http.HandleFunc("/install", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(400)
			return
		}
		pkgName := r.FormValue("package")
		pkgVer := r.FormValue("version")
		if pkgName == "" || pkgVer == "" {
			w.WriteHeader(400)
			return
		}
		
		pterm.Info.Printf("Installing %s@%s...\n", pkgName, pkgVer)
		if err := InstallPendingPlugin(projectRoot, pkgName, pkgVer); err != nil {
			utils.Error("Remote installation failed for %s@%s: %v", pkgName, pkgVer, err)
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}

		// Re-read config for fresh metadata
		content, _ := os.ReadFile(configPath)
		var freshConfig map[string]interface{}
		json.Unmarshal(content, &freshConfig)

		mu.Lock()
		var pReqs []PendingReq
		for _, p := range plugins {
			pReqs = append(pReqs, PendingReq{
				Name:       p.Name,
				Version:    p.Version,
				Identity:   p.Identity,
				Privileges: p.Privileges,
				Status:     p.Status,
			})
		}
		// Update status for the installed one first
		for i, pr := range pReqs {
			if pr.Name == pkgName {
				pReqs[i].Status = "pending"
			}
		}
		plugins = buildWebPlugins(projectRoot, pReqs, freshConfig, isReview)
		mu.Unlock()

		w.WriteHeader(200)
		pterm.Success.Printf("Installed %s@%s. Metadata refreshed.\n", pkgName, pkgVer)
		
		mu.Lock()
		b, _ := json.Marshal(plugins)
		mu.Unlock()
		broadcast(string(b))
	})

	http.HandleFunc("/install-batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var req struct {
			Packages []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"packages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			return
		}
		
		mu.Lock()
		failed := false
		for _, p := range req.Packages {
			pterm.Info.Printf("Installing %s@%s...\n", p.Name, p.Version)
			if err := InstallPendingPlugin(projectRoot, p.Name, p.Version); err != nil {
				utils.Error("Remote batch installation failed for %s@%s: %v", p.Name, p.Version, err)
				failed = true
				continue
			}
			pterm.Success.Printf("Batch installed %s@%s\n", p.Name, p.Version)
		}

		// Re-read config and refresh all
		content, _ := os.ReadFile(configPath)
		var freshConfig map[string]interface{}
		json.Unmarshal(content, &freshConfig)

		var pReqs []PendingReq
		for _, p := range plugins {
			status := p.Status
			// If it was just installed, mark as pending in the re-scan
			for _, ip := range req.Packages {
				if ip.Name == p.Name {
					status = "pending"
				}
			}
			pReqs = append(pReqs, PendingReq{
				Name:       p.Name,
				Version:    p.Version,
				Identity:   p.Identity,
				Privileges: p.Privileges,
				Status:     status,
			})
		}
		plugins = buildWebPlugins(projectRoot, pReqs, freshConfig, isReview)
		mu.Unlock()

		if failed {
			w.WriteHeader(207)
		} else {
			w.WriteHeader(200)
		}
		mu.Lock()
		b, _ := json.Marshal(plugins)
		mu.Unlock()
		broadcast(string(b))
	})

	http.HandleFunc("/cancel", func(w http.ResponseWriter, r *http.Request) {
		pterm.Info.Println("Manual session termination requested.")
		done <- true
	})

	srv := &http.Server{}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			pterm.Error.Printf("Server error: %v\n", err)
		}
	}()

	stop := utils.SignalManager.Subscribe()
	defer utils.SignalManager.Unsubscribe(stop)

	select {
	case <-stop:
		pterm.Info.Println("Verification cancelled via terminal.")
		broadcast("event: close\ndata: session-ended")
		time.Sleep(500 * time.Millisecond)
	case <-done:
		broadcast("event: close\ndata: session-ended")
		time.Sleep(500 * time.Millisecond)
	}

	area.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
