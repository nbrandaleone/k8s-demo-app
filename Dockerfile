FROM golang:1.15.5-alpine3.12 AS build

ENV CGO_ENABLED=0 \
    LANG=C.UTF-8

WORKDIR /src
RUN apk add --update --no-cache \
      curl \
      git \
      grep \
      make \
      ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /k8s-demo-app .

FROM scratch

ENV APP_UID=10000 \
    APP_GID=10000

COPY --from=build /k8s-demo-app /
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY static /static
COPY template.html /

USER ${APP_UID}:${APP_GID}

CMD ["/k8s-demo-app"]
