# Build static Linux binary, tiny runtime image for Fly.io.
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
ENV CGO_ENABLED=0
RUN go build -o /echo -trimpath -ldflags="-s -w" .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates \
	&& adduser -D -H -u 65532 appuser
COPY --from=build /echo /echo
USER appuser
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/echo"]
