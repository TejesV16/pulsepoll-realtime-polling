package repository

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"pulsepoll/backend/internal/models"
)

type Repository struct{ db *mongo.Database }

func New(db *mongo.Database) *Repository       { return &Repository{db: db} }
func (r *Repository) Users() *mongo.Collection { return r.db.Collection("users") }
func (r *Repository) Polls() *mongo.Collection { return r.db.Collection("polls") }
func (r *Repository) Votes() *mongo.Collection { return r.db.Collection("votes") }

func (r *Repository) CreateUser(ctx context.Context, u *models.User) error {
	_, err := r.Users().InsertOne(ctx, u)
	return err
}
func (r *Repository) FindUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := r.Users().FindOne(ctx, bson.M{"email": email}).Decode(&u)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
func (r *Repository) CreatePoll(ctx context.Context, p *models.Poll) error {
	_, err := r.Polls().InsertOne(ctx, p)
	return err
}
func (r *Repository) FindPoll(ctx context.Context, id primitive.ObjectID) (*models.Poll, error) {
	var p models.Poll
	err := r.Polls().FindOne(ctx, bson.M{"_id": id}).Decode(&p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
func (r *Repository) ListPollsByOwner(ctx context.Context, ownerID primitive.ObjectID) ([]models.Poll, error) {
	cur, err := r.Polls().Find(ctx, bson.M{"owner_id": ownerID}, options.Find().SetSort(bson.D{{"created_at", -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.Poll
	if err = cur.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []models.Poll{}
	}
	return out, nil
}
func (r *Repository) SetPollStatus(ctx context.Context, id, ownerID primitive.ObjectID, status string) error {
	res, err := r.Polls().UpdateOne(ctx, bson.M{"_id": id, "owner_id": ownerID}, bson.M{"$set": bson.M{"status": status}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}
func (r *Repository) DeletePoll(ctx context.Context, id, ownerID primitive.ObjectID) error {
	res, err := r.Polls().DeleteOne(ctx, bson.M{"_id": id, "owner_id": ownerID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	_, err = r.Votes().DeleteMany(ctx, bson.M{"poll_id": id})
	return err
}
func (r *Repository) CreateVote(ctx context.Context, v *models.Vote) error {
	_, err := r.Votes().InsertOne(ctx, v)
	return err
}
func (r *Repository) CountVotes(ctx context.Context, pollID primitive.ObjectID) (map[string]int64, error) {
	cur, err := r.Votes().Aggregate(ctx, mongo.Pipeline{{{"$match", bson.D{{"poll_id", pollID}}}}, {{"$group", bson.D{{"_id", "$option_id"}, {"count", bson.D{{"$sum", 1}}}}}}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := map[string]int64{}
	for cur.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		out[row.ID] = row.Count
	}
	return out, cur.Err()
}
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	_, err := r.Users().Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{"email", 1}}, Options: options.Index().SetUnique(true)})
	if err != nil {
		return err
	}
	_, err = r.Votes().Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{"poll_id", 1}, {"voter_hash", 1}}, Options: options.Index().SetUnique(true)})
	if err != nil {
		return err
	}
	_, err = r.Polls().Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{"owner_id", 1}, {"created_at", -1}}})
	return err
}
