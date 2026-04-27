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
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
)

//go:embed templates/plugin_verify.xfpml
var pluginVerifyTmpl string

var (
	clients   []chan string
	clientsMu sync.Mutex
)

func broadcast(msg string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for _, c := range clients {
		select {
		case c <- msg:
		default:
		}
	}
}

func HandleHtmlVerify(projectRoot string, pending []PendingReq, config map[string]interface{}, configPath string, isReview bool) {
	// Fetch master permissions for descriptions
	utils.Matrix("Fetching permission descriptions...")
	resp, err := http.Get("https://raw.githubusercontent.com/Nehonix-Team/XyPriss/master/.data/base/permissions.json")
	var masterPermissions map[string]struct {
		Name        string `json:"name"`
		Action      string `json:"action"`
		Description string `json:"description"`
	}
	if err == nil {
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&masterPermissions)
	}

	var plugins []WebPlugin
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

		// Get current permissions if in review mode or already authorized
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
		plugins = append(plugins, wp)
	}

	var mu sync.Mutex

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		pterm.Error.Printf("Failed to start local server: %v\n", err)
		return
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	pterm.Success.Printf("Verification dashboard started at %s\n", url)
	utils.OpenBrowser(url)

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

	http.HandleFunc("/apply", func(w http.ResponseWriter, r *http.Request) {
		var result map[string]string
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			http.Error(w, err.Error(), 400)
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

		for _, p := range currentPlugins {
			isTrusted := result["trust-"+p.Name] == "on"

			// If trust was unchecked and plugin was authorized, revoke it
			if !isTrusted {
				if isReview && p.Status == "authorized" {
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
				}
				continue
			}

			// Pre-initialize configuration block for this plugin
			pluginCfgRaw, ok := internal[p.Name]
			if !ok {
				pluginCfgRaw = make(map[string]interface{})
			}
			pluginCfg, ok := pluginCfgRaw.(map[string]interface{})
			if !ok {
				pluginCfg = make(map[string]interface{})
			}

			// Trust is checked: apply trust (update local map) + permissions
			if p.Identity != "" && p.Identity != "-" {
				// Update status in live list
				mu.Lock()
				for i, wp := range plugins {
					if wp.Name == p.Name {
						plugins[i].Status = "authorized"
					}
				}
				mu.Unlock()

				// Update sig in local map
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

			// Collect approved permissions from form
			var approved []string
			if p.Privileges != "" && p.Privileges != "none" {
				for _, id := range strings.Split(p.Privileges, ",") {
					id = strings.TrimSpace(id)
					perm := "perm-" + p.Name + "-" + id
					if result[perm] == "on" {
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
		}
		config["$internal"] = internal

		if out, err := json.MarshalIndent(config, "", "    "); err == nil {
			os.WriteFile(configPath, out, 0644)
			utils.Success("Remote authorization successful. Config updated.")
		}
		w.WriteHeader(200)
		done <- true

		// Broadcast update
		mu.Lock()
		b, _ := json.Marshal(plugins)
		mu.Unlock()
		broadcast(string(b))
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
			clientsMu.Unlock()
		}()

		for {
			select {
			case msg := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", msg)
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
		
		err := r.ParseForm()
		if err != nil {
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
					// Remove from list or mark as uninstalled
					plugins[i].Status = "NOT_INSTALLED"
				} else {
					plugins[i].Status = "pending"
				}
			}
		}
		mu.Unlock()

		w.WriteHeader(200)

		// Broadcast update
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

		if err := InstallPendingPlugin(projectRoot, pkgName, pkgVer); err != nil {
			utils.Error("Remote installation failed for %s@%s: %v", pkgName, pkgVer, err)
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}

		mu.Lock()
		for i, wp := range plugins {
			if wp.Name == pkgName {
				plugins[i].Status = "pending"
			}
		}
		mu.Unlock()

		w.WriteHeader(200)

		// Broadcast update
		mu.Lock()
		b, _ := json.Marshal(plugins)
		mu.Unlock()
		broadcast(string(b))
	})

	http.HandleFunc("/cancel", func(w http.ResponseWriter, r *http.Request) {
		done <- true
	})

	srv := &http.Server{}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			pterm.Error.Printf("Server error: %v\n", err)
		}
	}()

	pterm.Info.Println("Press Ctrl+C to cancel and return to terminal...")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stop:
		pterm.Info.Println("Verification cancelled.")
	case <-done:
		// Normal completion
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
