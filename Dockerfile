FROM golang:alpine as builder
WORKDIR /workdir
COPY go.mod /workdir
COPY go.sum /workdir
COPY main.go /workdir
RUN go build -o probeep
RUN strip probeep

FROM scratch
COPY --from=builder /workdir/probeep /
ENTRYPOINT ["/probeep"]