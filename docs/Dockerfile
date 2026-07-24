FROM ghcr.io/gohugoio/hugo:v0.164.0

WORKDIR /src

COPY --chown=hugo:hugo docs/go.mod docs/go.sum ./
RUN go mod download

COPY --chown=hugo:hugo docs/ ./

EXPOSE 1313

CMD ["server", "--bind", "0.0.0.0", "--port", "1313", "--baseURL", "http://localhost:1313/", "--disableFastRender", "--renderToMemory", "--noBuildLock"]
