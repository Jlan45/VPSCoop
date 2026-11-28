package main

import (
	"encoding/base64"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"vpscoop/internal/api"
	"vpscoop/internal/crypto"
	"vpscoop/internal/mcp"
	"vpscoop/internal/store"
)

//go:embed all:web/dist
var distFS embed.FS

func main() {
	dataDir := os.Getenv("VPSCOOP_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	dbPath := filepath.Join(dataDir, "vpscoop.db")
	var st *store.Store
	saltB64, verifier, _ := store.ReadMasterMeta(dbPath)
	if saltB64 != "" && verifier != "" {
		if envMaster := os.Getenv("VPSCOOP_MASTER_KEY"); envMaster != "" {
			if salt, decErr := base64.StdEncoding.DecodeString(saltB64); decErr == nil {
				if cipher, cErr := crypto.DeriveCipher(envMaster, salt); cErr == nil {
					if plain, vErr := cipher.Decrypt(verifier); vErr == nil && plain == "vpscoop-ok" {
						st, _ = store.New(dbPath, cipher)
					}
				}
			}
		}
	} else if crypto.HasLegacyKey(dataDir) {
		cipher, err := crypto.NewCipher(dataDir)
		if err != nil {
			log.Fatalf("init cipher: %v", err)
		}
		st, err = store.New(dbPath, cipher)
		if err != nil {
			log.Fatalf("init store: %v", err)
		}
	}
	if st != nil {
		defer st.Close()
	}

	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if st == nil {
			log.Fatalf("VPSCoop is locked or uninitialized; set VPSCOOP_MASTER_KEY (or VPSCOOP_SECRET_KEY) in env")
		}
		if err := mcp.ServeStdio(st); err != nil {
			log.Fatalf("mcp server error: %v", err)
		}
		return
	}

	mux := http.NewServeMux()
	srv := api.NewServer(st, dataDir)
	srv.RegisterRoutes(mux)

	subFS, err := fs.Sub(distFS, "web/dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(subFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			cleanPath := strings.TrimPrefix(r.URL.Path, "/")
			if cleanPath != "" {
				if _, err := fs.Stat(subFS, cleanPath); err == nil {
					if cleanPath == "sw.js" {
						w.Header().Set("Cache-Control", "no-cache")
					}
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			// SPA fallback to index.html
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	}

	addr := os.Getenv("VPSCOOP_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	log.Printf("VPSCoop server listening on http://localhost%s (data: %s)", addr, dataDir)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
