FROM golang:1.24 AS build

RUN useradd -u 10001 dimo

WORKDIR /go/src/github.com/DIMO-Network/oracle-example
COPY . /go/src/github.com/DIMO-Network/oracle-example/

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOFLAGS=-mod=vendor

RUN ls
RUN go mod tidy
RUN go mod vendor
RUN make install

FROM busybox AS package

LABEL maintainer="DIMO <hello@dimo.zone>"

WORKDIR /

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /go/src/github.com/DIMO-Network/oracle-example/target/bin/oracle-example .
COPY --from=build /go/src/github.com/DIMO-Network/oracle-example/internal/db/migrations ./internal/db/migrations

USER dimo

EXPOSE 8888

CMD /oracle-example
