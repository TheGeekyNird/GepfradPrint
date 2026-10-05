package api

import (
	"io/fs"
	"net/http"
)

func webFS() http.FileSystem { x, _ := fs.Sub(files, "assets"); return http.FS(x) }
