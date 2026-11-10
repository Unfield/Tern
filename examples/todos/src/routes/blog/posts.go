// Package blog lists the posts in src/content/blog.
package blog

import (
	"time"

	"github.com/Unfield/Tern/content"
	"github.com/Unfield/Tern/kit"
)

const Prerender = true

// Post is a blog post's front matter.
type Post struct {
	Title string    `yaml:"title" json:"title"`
	Date  time.Time `yaml:"date" json:"date"`
	Tags  []string  `yaml:"tags" json:"tags"`
}

// Posts is the blog's collection, newest first. The post page uses it too.
var Posts = &content.Collection[Post]{Dir: "blog", Sort: content.Newest(func(p Post) time.Time { return p.Date })}

type List struct {
	Posts []content.Summary[Post] `json:"posts"`
}

func Load(ctx *kit.Ctx) (List, error) {
	posts, err := Posts.Summaries(ctx)
	return List{Posts: posts}, err
}
