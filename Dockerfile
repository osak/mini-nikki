FROM golang:1.26-alpine AS builder
# scratch には CA 証明書が無いので、ここで入れたものを最終イメージにコピーする。
RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001

COPY . .
RUN templ generate
RUN CGO_ENABLED=0 go build -o mini-nikki .

FROM scratch
# リンクカードの OGP 取得で外部サイトに HTTPS で接続するため、証明書の検証に使う。
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /app
COPY --from=builder /app/mini-nikki .
EXPOSE 8080
CMD ["./mini-nikki"]
