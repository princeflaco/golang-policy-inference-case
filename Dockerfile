FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/infer ./cmd/infer

FROM public.ecr.aws/lambda/provided:al2023
COPY --from=build /out/infer /var/task/infer
ENTRYPOINT ["/var/task/infer"]
