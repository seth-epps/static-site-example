package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*
var templateFS embed.FS

//go:embed posts/*
var postFS embed.FS

const postPathSegment = "slug"

func main() {
	port := 8081
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	postFSSub, err := fs.Sub(postFS, "posts")
	if err != nil {
		// try not to panic, but this is good enough
		// to stop startup
		panic(err)
	}

	staticFSSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// try not to panic, but this is good enough
		// to stop startup
		panic(err)
	}

	postReader := NewFSPostReader(postFSSub)

	mux := http.NewServeMux()
	mux.Handle("GET /", http.HandlerFunc(rootHandler))
	mux.Handle("GET /posts/", http.HandlerFunc(postsHandler(postReader)))
	mux.Handle(fmt.Sprintf("GET /posts/{%s}", postPathSegment), http.HandlerFunc(postHandler(postReader)))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFSSub))))

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	slog.Info("starting server on :8081")
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("error starting server", "error", err.Error())
		os.Exit(1)
	}
}

func rootHandler(w http.ResponseWriter, req *http.Request) {
	switch req.URL.String() {
	case "/":
		// render the root template
		renderPageWithStatus(w, "root", http.StatusOK, nil)
	default:
		// render the not found template
		status := http.StatusNotFound
		renderPageWithStatus(w, "error", status, struct {
			Status int
		}{
			Status: status,
		})
	}
}

func postsHandler(postReader PostReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		posts, listErr := postReader.List()
		if listErr != nil {
			status := http.StatusInternalServerError
			renderPageWithStatus(w, "error", status, struct {
				Status int
			}{
				Status: status,
			})
		} else {
			renderPageWithStatus(w, "posts", http.StatusOK, struct {
				Posts []Post
			}{
				Posts: posts,
			})
		}
	}
}

func postHandler(postReader PostReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		slug := req.PathValue(postPathSegment)
		post, getErr := postReader.Get(slug)
		if getErr != nil {
			status := http.StatusInternalServerError
			if errors.Is(getErr, ErrPostNotFound) {
				status = http.StatusNotFound
			}

			renderPageWithStatus(w, "error", status, struct {
				Status int
			}{
				Status: status,
			})
		} else {
			renderPageWithStatus(w, "post", http.StatusOK, struct {
				Title   string
				Author  string
				Content template.HTML
			}{
				Title:   post.Title,
				Author:  post.Author,
				Content: template.HTML(post.Content),
			})
		}
	}
}

func renderPageWithStatus(w http.ResponseWriter, page string, status int, templateData any) {
	pageTemplate := fmt.Sprintf("%s.tmpl", page)
	templatePath := fmt.Sprintf("templates/%s", pageTemplate)
	tpl, err := template.ParseFS(templateFS, "templates/layout/*.tmpl", templatePath)
	if err != nil {
		slog.Error("failed to parse template", "page", page, "error", err.Error())
		http.Error(w, "Oh no!", http.StatusInternalServerError)
		return
	}

	buf := &bytes.Buffer{}
	err = tpl.ExecuteTemplate(buf, pageTemplate, templateData)
	if err != nil {
		slog.Error("failed to execute template", "page", page, "error", err.Error())
		http.Error(w, "Oh no!", http.StatusInternalServerError)
		return
	}

	// ignore errors writing to ResponseWriter
	w.WriteHeader(status)
	buf.WriteTo(w)
}
