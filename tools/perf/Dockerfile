FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/infer ./cmd/infer

FROM gcr.io/distroless/static-debian12
ENV HTTP_ADDR=:8080
EXPOSE 8080
COPY --from=build /out/infer /infer
ENTRYPOINT ["/infer"]
