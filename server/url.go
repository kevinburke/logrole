package server

import "strings"

type urlBuilder struct {
	basePath string
}

func (u urlBuilder) Path(p string) string {
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if u.basePath == "" {
		return p
	}
	if p == u.basePath || strings.HasPrefix(p, u.basePath+"/") {
		return p
	}
	if p == "/" {
		return u.basePath + "/"
	}
	return u.basePath + p
}

func (u urlBuilder) RequestURI(uri string) string {
	i := strings.IndexByte(uri, '?')
	if i == -1 {
		return u.Path(uri)
	}
	return u.Path(uri[:i]) + uri[i:]
}

func optionalBasePath(basePaths []string) string {
	if len(basePaths) == 0 {
		return ""
	}
	return basePaths[0]
}
