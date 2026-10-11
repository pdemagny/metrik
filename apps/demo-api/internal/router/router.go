package router

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/loopfz/gadgeto/tonic"
	"github.com/wI2L/fizz"
	"github.com/wI2L/fizz/openapi"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	APIResponse struct {
		Message string `json:"message" validate:"required,max=255"`
		Status  int    `json:"status" validate:"required,max=50"`
		Reason  string `json:"reason,omitempty" validate:"max=255"`
	}
)

// NewRouter returns the API: health, slow and readiness endpoints under /v1,
// and the OpenAPI document at /openapi.
func NewRouter() (*fizz.Fizz, error) {
	zapConfig := zap.NewProductionConfig()
	zapConfig.DisableCaller = true
	zapConfig.DisableStacktrace = true
	zapConfig.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	logger, err := zapConfig.Build()
	if err != nil {
		return nil, err
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(ginzap.Ginzap(logger, time.RFC3339, true))
	router.Use(ginzap.RecoveryWithZap(logger, true))

	f := fizz.NewFromEngine(router)

	infos := &openapi.Info{
		Title:       "Demo API",
		Description: "Demo API with live, slow and flaky endpoints, meant as a target for health checks.",
		Version:     "0.1.0",
	}
	f.GET("/openapi", nil, f.OpenAPI(infos, "json"))

	routes(f.Group("/v1", "v1 api", "Demo API v1."))

	if len(f.Errors()) != 0 {
		return nil, fmt.Errorf("fizz errors: %v", f.Errors())
	}
	return f, nil
}

func routes(grp *fizz.RouterGroup) {
	grp.GET("/.well-known/live", []fizz.OperationOption{
		fizz.Summary("Liveness check"),
		fizz.Description("Indicates if the service is running and responsive to basic HTTP requests"),
	}, tonic.Handler(getLive, http.StatusOK))
	grp.GET("/slow", []fizz.OperationOption{
		fizz.Summary("Slow endpoint"),
		fizz.Description("Volontarily slow endpoint"),
	}, tonic.Handler(getSlow, http.StatusOK))
	grp.GET("/.well-known/ready", []fizz.OperationOption{
		fizz.Summary("Readyness check"),
		fizz.Description("Indicates if service has completed its startup routines and is prepared to accept user traffic"),
		fizz.Response("503", "Service Unavailable", nil, nil, nil),
		fizz.Header("Content-Type", "application/json", APIResponse{}),
	}, tonic.Handler(getReady, http.StatusOK))
}

func getLive(_ *gin.Context) (*APIResponse, error) {
	return &APIResponse{Message: "liveness", Status: http.StatusOK}, nil
}

func getSlow(_ *gin.Context) (*APIResponse, error) {
	time.Sleep(time.Duration(rand.IntN(5)) * time.Second)
	return &APIResponse{Message: "slow", Status: http.StatusOK}, nil
}

func getReady(_ *gin.Context) (*APIResponse, error) {
	reason := ""
	status := http.StatusOK
	if !mayhem() {
		reason = "database service not ready yet"
		status = http.StatusServiceUnavailable
	}
	return &APIResponse{Message: "readiness", Status: status, Reason: reason}, nil
}

func mayhem() bool {
	num := rand.IntN(100)
	return num%2 == 0
}
