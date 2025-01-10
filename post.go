package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/parser"
	"go.abhg.dev/goldmark/frontmatter"
)

var ErrPostNotFound = errors.New("post not found")

type Post struct {
	// Slug is a user-friendly path segment
	Slug string `yaml:"slug"`
	// Title is the post title
	Title string `yaml:"title"`
	// Description is a short description of the post content
	Description string `yaml:"description"`
	// Author is the post author
	Author string `yaml:"author"`
	// Date is the date the post was created
	Date string `yaml:"date"`
	// Content is the raw string content of the post
	Content string
}

type PostReader interface {
	List() ([]Post, error)
	Get(slug string) (Post, error)
}

type FSPostReader struct {
	fsys fs.FS
	md   goldmark.Markdown
}

func NewFSPostReader(fsys fs.FS) *FSPostReader {
	return &FSPostReader{
		fsys: fsys,
		md: goldmark.New(
			goldmark.WithExtensions(
				highlighting.NewHighlighting(
					highlighting.WithStyle("solarized-dark"),
					highlighting.WithFormatOptions(
						chromahtml.TabWidth(8),
						chromahtml.WithLineNumbers(true),
					),
				),
				&frontmatter.Extender{},
			),
		),
	}
}

func (pr FSPostReader) List() ([]Post, error) {
	files, err := fs.Glob(pr.fsys, "*.md")
	if err != nil {
		return nil, fmt.Errorf("failed to glob posts: %w", err)
	}

	var posts []Post

	for _, path := range files {
		_, slug := filepath.Split(path)
		slug, _ = strings.CutSuffix(slug, ".md")
		post, err := pr.readPostFromFile(path)
		if err != nil {
			slog.Warn("could not decode post content", "slug", slug, "error", err.Error())
			continue
		}
		post.Slug = slug
		posts = append(posts, post)
	}
	return posts, nil
}

func (pr FSPostReader) Get(slug string) (Post, error) {
	path := fmt.Sprintf("%s.md", slug)
	switch post, err := pr.readPostFromFile(path); {
	// error wrapping handles the filesystem errors
	case err != nil && errors.Is(err, fs.ErrNotExist):
		return post, ErrPostNotFound
	case err != nil:
		return post, fmt.Errorf("failed to retrieve post %s: %w", slug, err)
	default:
		post.Slug = slug
		return post, nil
	}
}

func (pr FSPostReader) readPostFromFile(path string) (Post, error) {
	f, err := pr.fsys.Open(path)
	if err != nil {
		return Post{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return Post{}, fmt.Errorf("failed to read file content: %w", err)
	}

	post, err := pr.decodePost(b)
	if err != nil {
		return Post{}, fmt.Errorf("failed to decode post content: %w", err)
	}
	return post, nil
}

func (pr FSPostReader) decodePost(b []byte) (Post, error) {
	post := Post{}
	var buf bytes.Buffer
	parseCtx := parser.NewContext()
	if err := pr.md.Convert(b, &buf, parser.WithContext(parseCtx)); err != nil {
		return post, fmt.Errorf("failed to parse content: %w", err)
	}

	// Decode frontmatter meta into post then attach content
	metaDecoder := frontmatter.Get(parseCtx)
	if err := metaDecoder.Decode(&post); err != nil {
		return post, fmt.Errorf("could not decode metadata: %w", err)
	}
	post.Content = buf.String()
	return post, nil
}
