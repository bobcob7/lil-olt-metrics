FROM node:24-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BRANCH=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/query/dashboard/dist ./internal/query/dashboard/dist
RUN CGO_ENABLED=0 go build \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.branch=${BRANCH}" \
    -o /lil-olt-metrics ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /lil-olt-metrics /lil-olt-metrics

EXPOSE 4317 4318 9090

ENTRYPOINT ["/lil-olt-metrics"]
