FROM golang:1.26-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/a-h/templ/cmd/templ@latest

COPY . .
RUN templ generate
RUN CGO_ENABLED=0 go build -o mini-nikki .

FROM scratch
WORKDIR /app
COPY --from=builder /app/mini-nikki .
EXPOSE 8080
CMD ["./mini-nikki"]
