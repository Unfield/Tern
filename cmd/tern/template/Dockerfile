# syntax=docker/dockerfile:1

# Build stage: Node builds the client and the server renderer, Go builds
# the binary that embeds them.
FROM golang:1.27-bookworm AS build
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && apt-get install -y --no-install-recommends nodejs
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY package.json package-lock.json* pnpm-lock.yaml* ./
RUN if [ -f pnpm-lock.yaml ]; then corepack enable && pnpm install --frozen-lockfile; \
    elif [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY . .
RUN CGO_ENABLED=0 go tool tern build -o /out/app

# Runtime stage: a static binary needs no OS packages.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
ENV PORT=3000
EXPOSE 3000
USER nonroot:nonroot
ENTRYPOINT ["/app"]
