FROM golang:1.13 AS builder

LABEL maintainer="nosql-team@criteo.com"


ENV SKIP_TEST=${SKIP_TEST:+true}

WORKDIR /app

# Cache depenencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the project
COPY . .
RUN mkdir bin ; cd bin ; \
    CGO_ENABLED=0 GOOS=linux go build -gcflags "all=-N -l" ../

RUN [ -z "$SKIP_TEST" ] && rm -rf app/ && go test ./...

# Compile Delve debugger
# RUN go get -u github.com/go-delve/delve/cmd/dlv



FROM alpine:3.11

LABEL maintener="nosql-team@criteo.com"
COPY --from=builder /app/bin/ .
#COPY --from=builder /go/bin/dlv .
WORKDIR /

RUN apk add --no-cache libc6-compat

CMD ["/bin/sh", "-c", "./espoke"]
