# lp-server control plane image
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lp-server ./cmd/lp-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lp-server /lp-server
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/lp-server"]
