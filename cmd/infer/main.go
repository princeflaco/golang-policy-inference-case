package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/princeflaco/golang-policy-inference-case/internal/httpapi"
)

func main() {
	lambda.Start(httpapi.Handle)
}
