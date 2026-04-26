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
			Status:   p.Status,
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
			// In review mode, if trust checkbox was UNCHECKED, it means REVOKE
			if result["trust-"+p.Name] != "on" {
				if isReview && p.Status == "authorized" {
					err := RevokeTrust(projectRoot, p.Name, false)
					if err != nil {
						utils.Error("Failed to revoke trust for %s: %v", p.Name, err)
					}
					delete(internal, p.Name)
				}
				continue
			}

			if result["trust-"+p.Name] == "on" {
				// Validate trust using the same backend logic
				err := TrustPlugin(projectRoot, p.Name, p.Identity)
				if err != nil {
					utils.Error("Failed integrity check for %s. Skipping: %v", p.Name, err)
					continue
				}
			}

			pluginCfgRaw, ok := internal[p.Name]
			if !ok {
				pluginCfgRaw = make(map[string]interface{})
			}
			pluginCfg, ok := pluginCfgRaw.(map[string]interface{})
			if !ok {
				pluginCfg = make(map[string]interface{})
			}

			// Signature is already handled by TrustPlugin if we just activated it.
			// But for pre-authorized cases where we're just updating permissions, 
			// the signature is already in `$internal`.
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

			// In review mode, we ALWAYS update the permissions block, even if empty
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
		w.WriteHeader(200)
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

		w.WriteHeader(200)
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
