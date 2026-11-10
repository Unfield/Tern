// Package blog reads the posts in src/content/posts.
package blog

import (
	"time"

	"github.com/Unfield/Tern/content"
)

// Post is a post's front matter.
type Post struct {
	Title       string    `yaml:"title" json:"title"`
	Description string    `yaml:"description" json:"description"`
	Date        time.Time `yaml:"date" json:"date"`
}

// Posts are the blog's posts, newest first.
var Posts = &content.Collection[Post]{
	Dir:  "posts",
	Sort: content.Newest(func(p Post) time.Time { return p.Date }),
}
