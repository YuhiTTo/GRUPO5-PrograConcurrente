package persistencia

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// DocumentoPrediccion representa una predicción almacenada.
type DocumentoPrediccion struct {
	OCID             string `bson:"ocid" json:"ocid"`
	Metodo           string `bson:"metodo" json:"metodo"`
	Categoria        string `bson:"categoria" json:"categoria"`
	FechaPublicacion string `bson:"fecha_publicacion" json:"fecha_publicacion"`
	BuyerID          string `bson:"buyer_id" json:"buyer_id"`
	MetodoDetalle    string `bson:"metodo_detalle" json:"metodo_detalle"`
	Titulo           string `bson:"titulo" json:"titulo"`
	Descripcion      string `bson:"descripcion" json:"descripcion"`

	Probabilidad   float64 `bson:"probabilidad" json:"probabilidad"`
	ClasePredicha  uint8   `bson:"clase_predicha" json:"clase_predicha"`
	Umbral         float64 `bson:"umbral" json:"umbral"`
	Interpretacion string  `bson:"interpretacion" json:"interpretacion"`
	Modelo         string  `bson:"modelo" json:"modelo"`

	FechaConsulta time.Time `bson:"fecha_consulta" json:"fecha_consulta"`
}

// Almacen administra la conexión y colección de MongoDB.
type Almacen struct {
	cliente   *mongo.Client
	coleccion *mongo.Collection
}

// Conectar abre una conexión autenticada con MongoDB.
func Conectar(ctx context.Context) (*Almacen, error) {
	usuario := os.Getenv("MONGO_ROOT_USER")
	clave := os.Getenv("MONGO_ROOT_PASSWORD")

	if strings.TrimSpace(usuario) == "" || clave == "" {
		return nil, fmt.Errorf("faltan credenciales de MongoDB")
	}

	opciones := options.Client().
		ApplyURI("mongodb://mongo:27017").
		SetAuth(options.Credential{
			Username:   usuario,
			Password:   clave,
			AuthSource: "admin",
		})

	cliente, err := mongo.Connect(opciones)
	if err != nil {
		return nil, fmt.Errorf("error creando conexión MongoDB: %w", err)
	}

	// Verificar que el servidor responde realmente.
	if err := cliente.Ping(ctx, readpref.Primary()); err != nil {
		_ = cliente.Disconnect(context.Background())
		return nil, fmt.Errorf("error conectando a MongoDB: %w", err)
	}

	coleccion := cliente.
		Database("compras_publicas_pc4").
		Collection("predicciones")

	return &Almacen{
		cliente:   cliente,
		coleccion: coleccion,
	}, nil
}

// Guardar inserta una predicción y devuelve su identificador.
func (a *Almacen) Guardar(
	ctx context.Context,
	documento DocumentoPrediccion,
) (string, error) {
	resultado, err := a.coleccion.InsertOne(ctx, documento)
	if err != nil {
		return "", fmt.Errorf("error guardando predicción: %w", err)
	}

	id, ok := resultado.InsertedID.(bson.ObjectID)
	if !ok {
		return "", fmt.Errorf("MongoDB devolvió un ID inesperado")
	}

	return id.Hex(), nil
}

// Cerrar libera la conexión al detener la API.
func (a *Almacen) Cerrar(ctx context.Context) error {
	if a == nil || a.cliente == nil {
		return nil
	}

	ctx, cancelar := context.WithTimeout(ctx, 5*time.Second)
	defer cancelar()

	return a.cliente.Disconnect(ctx)
}

// BuscarPorID recupera una predicción almacenada en MongoDB.
func (a *Almacen) BuscarPorID(
	ctx context.Context,
	id string,
) (*DocumentoPrediccion, error) {

	// Convertir el identificador hexadecimal de MongoDB.
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf(
			"identificador de predicción inválido: %w",
			err,
		)
	}

	var documento DocumentoPrediccion

	// Buscar el documento por su identificador.
	err = a.coleccion.FindOne(
		ctx,
		bson.M{"_id": objectID},
	).Decode(&documento)

	if err != nil {
		return nil, fmt.Errorf(
			"error consultando predicción: %w",
			err,
		)
	}

	return &documento, nil
}
