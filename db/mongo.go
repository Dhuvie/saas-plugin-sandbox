package db

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoRepository struct {
	client     *mongo.Client
	db         *mongo.Database
	plugins    *mongo.Collection
	executions *mongo.Collection
}

func NewMongoRepository(uri string, dbName string) (*MongoRepository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(ctx)
		return nil, fmt.Errorf("failed to ping mongodb: %w", err)
	}

	database := client.Database(dbName)
	return &MongoRepository{
		client:     client,
		db:         database,
		plugins:    database.Collection("plugins"),
		executions: database.Collection("executions"),
	}, nil
}

func (r *MongoRepository) SavePlugin(plugin *Plugin) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.plugins.InsertOne(ctx, plugin)
	return err
}

func (r *MongoRepository) GetPlugin(id string) (*Plugin, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var plugin Plugin
	err := r.plugins.FindOne(ctx, bson.M{"_id": id}).Decode(&plugin)
	if err == mongo.ErrNoDocuments {
		return nil, fmt.Errorf("plugin not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return &plugin, nil
}

func (r *MongoRepository) ListPlugins() ([]*Plugin, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cursor, err := r.plugins.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var plugins []*Plugin
	if err := cursor.All(ctx, &plugins); err != nil {
		return nil, err
	}
	if plugins == nil {
		return []*Plugin{}, nil
	}
	return plugins, nil
}

func (r *MongoRepository) UpdatePluginStatus(id string, status string, compiledWasm []byte, compileErrors string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"status":         status,
			"compiled_wasm":  compiledWasm,
			"compile_errors": compileErrors,
		},
		"$inc": bson.M{
			"version": 1,
		},
	}

	_, err := r.plugins.UpdateByID(ctx, id, update)
	return err
}

func (r *MongoRepository) UpdatePluginCode(id string, name string, sourceCode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"name":           name,
			"source_code":    sourceCode,
			"status":         "pending",
			"compile_errors": "",
		},
	}

	_, err := r.plugins.UpdateByID(ctx, id, update)
	return err
}

func (r *MongoRepository) SaveExecution(exec *Execution) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.executions.InsertOne(ctx, exec)
	return err
}

func (r *MongoRepository) GetExecutions(pluginID string) ([]*Execution, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cursor, err := r.executions.Find(ctx, bson.M{"plugin_id": pluginID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var executions []*Execution
	if err := cursor.All(ctx, &executions); err != nil {
		return nil, err
	}
	if executions == nil {
		return []*Execution{}, nil
	}
	return executions, nil
}

func (r *MongoRepository) DeletePlugin(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.executions.DeleteMany(ctx, bson.M{"plugin_id": id})
	if err != nil {
		return err
	}
	_, err = r.plugins.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *MongoRepository) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return r.client.Disconnect(ctx)
}
