package ags

import (
	"net/http"
	"os"
	"path/filepath"
)

// FileServerOption defines options for file server configuration
type FileServerOption func(*fileServerConfig)

type fileServerConfig struct {
	serveSPA  bool
	indexFile string
	distPath  string
}

// WithSPASupport enables Single Page Application support
func WithSPASupport(enable bool) FileServerOption {
	return func(f *fileServerConfig) {
		f.serveSPA = enable
	}
}

// WithIndexFile sets a custom index file
func WithIndexFile(filename string) FileServerOption {
	return func(f *fileServerConfig) {
		f.indexFile = filename
	}
}

// RegisterFileServer adds a catch-all route for serving static files
func (h *Handler) RegisterFileServer(distPath string, opts ...FileServerOption) error {
	// Clean and verify the dist path
	absPath, err := filepath.Abs(distPath)
	if err != nil {
		return NewError(ErrCodeInternal, "Invalid dist path").WithError(err)
	}

	// Verify directory exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return NewError(ErrCodeNotFound, "Distribution directory not found").WithError(err)
	}

	// Initialize default config
	config := &fileServerConfig{
		serveSPA:  true,
		indexFile: "index.html",
		distPath:  absPath,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Verify index file exists if SPA mode is enabled
	if config.serveSPA {
		indexPath := filepath.Join(absPath, config.indexFile)
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			return NewError(ErrCodeNotFound, "Index file not found").
				WithError(err).
				WithMetadata("path", indexPath)
		}
	}

	// Store config and handler for later use
	h.fileServer = config
	h.staticHandler = http.FileServer(http.Dir(absPath))
	return nil
}
