# DEP-1: static Go binary on a distroless base.
# DEP-7: the build stage runs the same make targets as CI and local development.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN make lint vet test build BIN=/out/ddia VERSION=${VERSION} \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ddia /ddia
COPY --from=build --chown=nonroot:nonroot /out/data /data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/ddia"]
CMD ["serve"]
