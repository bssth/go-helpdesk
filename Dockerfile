FROM golang:1.20 AS builder
WORKDIR /
COPY . /
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app ./main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app/
COPY --from=builder ./app ./
CMD ["./app"]