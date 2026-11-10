// The app's base path, for components that build URLs outside $routes
// (such as <Image>). The router sets it in the browser and the renderer on
// the server, so both render the same URLs.
let base = '';

export function setBase(b: string) {
	base = b;
}

export function getBase(): string {
	return base;
}
