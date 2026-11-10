package routes

import "github.com/Unfield/Tern/kit"

type Data struct {
	Greeting string `json:"greeting"`
}

func Load(*kit.Ctx) (Data, error) { return Data{Greeting: "Hello from the server."}, nil }
