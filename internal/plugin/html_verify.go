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
	"syscall"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
)

//go:embed templates/plugin_verify.xfpml
var pluginVerifyTmpl string

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
		wp := WebPlugin{
			Name:     p.Name,
			Identity: p.Identity,
		}

		// Get current permissions if in review mode
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
					Approved:    currentPerms[id] || !isReview, // Auto-approve if not review mode (pending list)
				})
			}
		}
		plugins = append(plugins, wp)
	}

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

		for _, p := range pending {
			// Check if trust checkbox was checked: "trust-<name>"
			if result["trust-"+p.Name] != "on" {
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

			// Permissions: "perm-<plugin>-<id>"
			var approved []string
			if p.Privileges != "" && p.Privileges != "none" {
				for _, id := range strings.Split(p.Privileges, ",") {
					id = strings.TrimSpace(id)
					if result["perm-"+p.Name+"-"+id] == "on" {
						approved = append(approved, id)
					}
				}
			}

			if len(approved) > 0 {
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
			}
			internal[p.Name] = pluginCfg
		}
		config["$internal"] = internal

		if out, err := json.MarshalIndent(config, "", "    "); err == nil {
			os.WriteFile(configPath, out, 0644)
			utils.Success("Remote authorization successful. Config updated.")
		}
		w.WriteHeader(200)
		done <- true
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
