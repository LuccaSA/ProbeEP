FROM golang:alpine as builder
WORKDIR /workdir
RUN apk add --no-cache binutils
COPY go.mod /workdir
COPY go.sum /workdir
COPY main.go /workdir
RUN go build -o probeep
RUN strip probeep

FROM alpine
COPY --from=builder /workdir/probeep /
ENTRYPOINT ["/probeep"]