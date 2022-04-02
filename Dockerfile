FROM golang:1.16-alpine
# Set destination for COPY
RUN apk update
RUN apk add --virtual build-dependencies  build-base gcc

WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .

RUN go build -o ./app/main

# Build
EXPOSE 8080
CMD ["./app/main"]