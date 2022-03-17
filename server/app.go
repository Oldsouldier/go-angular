package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
)

type student struct {
	ID   string `gorm:"primary_key" json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type App struct {
	db *gorm.DB
	r  *mux.Router
}

func (a *App) start() {
	ctx := context.Background()
	exp, err := newExporter(ctx)
	if err != nil {
		log.Fatalf("failed to initialize exporter: %v", err)
	}

	// Create a new tracer provider with a batch span processor and the otlp exporter.
	tp := newTraceProvider(exp)

	// Handle this error in a sensible manner where possible
	defer func() { _ = tp.Shutdown(ctx) }()

	// Set the Tracer Provider and the W3C Trace Context propagator as globals
	otel.SetTracerProvider(tp)

	// Register the trace context and baggage propagators so data is propagated across services/processes.
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	a.db.AutoMigrate(&student{})

	a.r.Handle("/students", otelhttp.NewHandler(http.HandlerFunc(a.getAllStudents), "get students")).Methods("GET")
	a.r.Handle("/students", otelhttp.NewHandler(http.HandlerFunc(a.addStudent), "post students")).Methods("POST")
	a.r.Handle("/students/{id}", otelhttp.NewHandler(http.HandlerFunc(a.updateStudent), "update student")).Methods("PUT")
	a.r.Handle("/students/{id}", otelhttp.NewHandler(http.HandlerFunc(a.deleteStudent), "delete student")).Methods("DELETE")
	a.r.PathPrefix("/").Handler(http.FileServer(http.Dir("./webapp/dist/webapp/")))
	log.Fatal(http.ListenAndServe(":8080", a.r))
}

func (a *App) getAllStudents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var all []student
	err := a.db.Find(&all).Error
	if err != nil {
		sendErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	err = json.NewEncoder(w).Encode(all)
	if err != nil {
		sendErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) addStudent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var s student
	err := json.NewDecoder(r.Body).Decode(&s)
	if err != nil {
		sendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.ID = uuid.New().String()
	err = a.db.Save(&s).Error
	if err != nil {
		sendErr(w, http.StatusInternalServerError, err.Error())
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

func (a *App) updateStudent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var s student
	err := json.NewDecoder(r.Body).Decode(&s)
	if err != nil {
		sendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.ID = mux.Vars(r)["id"]
	err = a.db.Save(&s).Error
	if err != nil {
		sendErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) deleteStudent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	err := a.db.Unscoped().Delete(student{ID: mux.Vars(r)["id"]}).Error
	if err != nil {
		sendErr(w, http.StatusInternalServerError, err.Error())
	}
}

func sendErr(w http.ResponseWriter, code int, message string) {
	resp, _ := json.Marshal(map[string]string{"error": message})
	http.Error(w, string(resp), code)
}

// Otel -----------------

func newExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	// Configuration to export data to Honeycomb:
	//
	// 1. The Honeycomb endpoint
	// 2. Your API key, set as the x-honeycomb-team header
	// 3. A Dataset name, set as the x-honeycomb-dataset header
	//
	// A Dataset is a bucket where data gets stored in Honeycomb.
	opts := []otlptracegrpc.Option{
		//otlptracegrpc.WithEndpoint("api.honeycomb.io:443"),
		//otlptracegrpc.WithHeaders(map[string]string{
		//	"x-honeycomb-team":    os.Getenv("HONEYCOMB_API_KEY"),
		//	"x-honeycomb-dataset": os.Getenv("HONEYCOMB_DATASET"),
		//}),
		//otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint("otel-collector:4317"),
	}

	client := otlptracegrpc.NewClient(opts...)
	return otlptrace.New(ctx, client)
}

func newTraceProvider(exp *otlptrace.Exporter) *sdktrace.TracerProvider {
	// The service.name attribute is required.
	resource :=
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("ExampleService"),
		)

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource),
	)
}
