package serve

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http" 
	"os"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/utils"
)

type FileItem struct {
	Name  string
	Path  string
	IsDir bool
	Size  int64
}

func StartServer(port int, dir string, tmplContent string) error {
	tmpl, err := template.New("index").Parse(tmplContent)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		tmpl.Execute(w, nil)
	})

	mux.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		targetDir := r.URL.Query().Get("dir")
		if targetDir == "" {
			targetDir = "."
		}

		// Prevent directory traversal
		if strings.Contains(targetDir, "..") {
			http.Error(w, "Invalid directory", http.StatusBadRequest)
			return
		}

		fullPath := filepath.Join(dir, targetDir)
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			http.Error(w, "Failed to read directory", http.StatusInternalServerError)
			return
		}
		
		var files []FileItem
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			
			relPath := filepath.Join(targetDir, entry.Name())
			relPath = strings.ReplaceAll(relPath, "\\", "/")
			if relPath == "./" || relPath == "." {
				continue
			}
			relPath = strings.TrimPrefix(relPath, "./")

			files = append(files, FileItem{
				Name:  entry.Name(),
				Path:  relPath,
				IsDir: entry.IsDir(),
				Size:  info.Size(),
			})
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(files)
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		query := strings.ToLower(r.URL.Query().Get("q"))
		if query == "" {
			json.NewEncoder(w).Encode([]FileItem{})
			return
		}

		var results []FileItem
		
		var searchDir func(currentPath string, depth int)
		searchDir = func(currentPath string, depth int) {
			if depth > 3 {
				return
			}
			
			entries, err := os.ReadDir(filepath.Join(dir, currentPath))
			if err != nil {
				return
			}
			
			for _, entry := range entries {
				if entry.Name() == ".git" || entry.Name() == "node_modules" {
					continue
				}

				relPath := filepath.Join(currentPath, entry.Name())
				relPath = strings.ReplaceAll(relPath, "\\", "/")
				
				if strings.Contains(strings.ToLower(entry.Name()), query) {
					info, err := entry.Info()
					size := int64(0)
					if err == nil {
						size = info.Size()
					}
					results = append(results, FileItem{
						Name:  entry.Name(),
						Path:  relPath,
						IsDir: entry.IsDir(),
						Size:  size,
					})
				}
				
				if entry.IsDir() {
					searchDir(relPath, depth+1)
				}
			}
		}

		searchDir("", 0)
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	fileServer := http.FileServer(http.Dir(dir))
	mux.HandleFunc("/raw/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/raw")
		fileServer.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		utils.Warn("Port %d in use, finding available port...", port)
		listener, err = net.Listen("tcp", ":0")
		if err != nil {
			return err
		}
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	utils.Success("XFPM Server started successfully!")
	// fmt.Printf("➜  Local:   http://localhost:%d\n", actualPort)
	fmt.Printf("➜  Serving HTTP on :: port %d (http://[::]:%d/)\n", actualPort, actualPort)
	fmt.Printf("➜  Serving: %s\n", dir)
	fmt.Println("Press Ctrl+C to stop.")

	return http.Serve(listener, mux)
}
