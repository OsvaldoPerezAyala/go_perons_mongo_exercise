package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Persona struct {
	Nombres         string `json:"nombres"`
	ApellidoPaterno string `json:"apellido_paterno"`
	ApellidoMaterno string `json:"apellido_materno"`
	CURP            string `json:"curp"`
	Edad            int    `json:"edad"`
	FechaNacimiento string `json:"fecha_nacimiento"`
	Genero          string `json:"genero"`
	Matricula       int    `json:"matricula"`
}

var client *mongo.Client

func main() {

	mongoURI := "mongodb://osvaldo:mongo_pass@localhost:27018/admin?retryWrites=true&serverSelectionTimeoutMS=5000&connectTimeoutMS=10000&authSource=admin&authMechanism=SCRAM-SHA-256"
	clientOptions := options.Client().ApplyURI(mongoURI)

	var err error
	client, err = mongo.NewClient(clientOptions)
	if err != nil {
		log.Fatalf("Error al crear el cliente de MongoDB: %v", err)
	}

	// Conectar al servidor MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = client.Connect(ctx)
	if err != nil {
		log.Fatalf("Error al conectar con el servidor MongoDB: %v", err)
	}

	// Verificar la conexión
	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatalf("Error al hacer ping al servidor MongoDB: %v", err)
	} else {
		log.Println("Conexión exitosa a MongoDB")
	}

	http.HandleFunc("/persona", personaHandler)
	http.HandleFunc("/personas", getPersonasHandler)
	http.ListenAndServe(":8080", nil)
}

func personaHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		personaHandlerPost(w, r)
	case "GET":
		getPersonaHandler(w, r)
	default:
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
	}
}

func personaHandlerPost(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodPost {
		decoder := json.NewDecoder(r.Body)

		var p Persona
		err := decoder.Decode(&p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = guardarPersona(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	} else {
		http.Error(w, "Método no soportado", http.StatusMethodNotAllowed)
	}
}

func guardarPersona(p Persona) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collection := client.Database("persona_go").Collection("personas")

	filter := bson.M{"curp": p.CURP}
	var existingPersona Persona
	err := collection.FindOne(ctx, filter).Decode(&existingPersona)

	if err == nil {
		return fmt.Errorf("La CURP %s ya existe en la base de datos", p.CURP)
	} else if err != mongo.ErrNoDocuments {
		return err
	}

	p.Edad, p.FechaNacimiento, p.Genero = obtenerInformacionCURP(p.CURP)
	p.Matricula = generarCodigoAleatorio()

	_, err = collection.InsertOne(ctx, p)
	return err
}

func obtenerInformacionCURP(curp string) (int, string, string) {
	year := curp[4:6]
	month := curp[6:8]
	day := curp[8:10]
	sexChar := curp[10]

	// Calcular fecha de nacimiento
	currentYear := time.Now().Year()
	birthYear, _ := strconv.Atoi("20" + year)
	if currentYear-2000 < birthYear {
		birthYear, _ = strconv.Atoi("19" + year)
	}
	birthDate := fmt.Sprintf("%s-%s-%s", strconv.Itoa(birthYear), month, day)

	// Calcular edad
	age := currentYear - birthYear

	// Obtener género
	gender := "Desconocido"
	if sexChar == 'H' {
		gender = "Hombre"
	} else if sexChar == 'M' {
		gender = "Mujer"
	}

	return age, birthDate, gender
}

func generarCodigoAleatorio() int {
	rand.Seed(time.Now().UnixNano())
	min := 1000000000 // límite inferior de 10 dígitos
	max := 9999999999 // límite superior de 10 dígitos
	return rand.Intn(max-min+1) + min
}

func getPersonasHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	pageQuery := r.URL.Query().Get("page")
	perPageQuery := r.URL.Query().Get("perPage")

	page, _ := strconv.Atoi(pageQuery)
	perPage, _ := strconv.Atoi(perPageQuery)

	if page < 1 {
		page = 1
	}

	if perPage < 1 {
		perPage = 10
	}

	skip := (page - 1) * perPage

	collection := client.Database("persona_go").Collection("personas")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var personas []Persona
	cursor, err := collection.Find(ctx, bson.M{}, options.Find().SetSkip(int64(skip)).SetLimit(int64(perPage)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for cursor.Next(ctx) {
		var p Persona
		cursor.Decode(&p)
		personas = append(personas, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(personas)
}

func getPersonaHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	matriculaQuery, _ := strconv.Atoi(r.URL.Query().Get("matricula"))
	curpQuery := r.URL.Query().Get("curp")

	var filter bson.M
	if matriculaQuery != 0 {
		filter = bson.M{"matricula": matriculaQuery}
	} else if curpQuery != "" {
		filter = bson.M{"curp": curpQuery}
	} else {
		http.Error(w, "Debes proporcionar matricula o curp", http.StatusBadRequest)
		return
	}

	collection := client.Database("persona_go").Collection("personas")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var persona Persona
	err := collection.FindOne(ctx, filter).Decode(&persona)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "No se encontró la persona", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(persona)
}
