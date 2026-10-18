package member

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kaderisasi/web/internal/auth"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/google"
)

// Register is AuthController.register's transaction: the user (password
// hashed by the AuthFinder mixin), its member id from a second save, and the
// profile, all committed together. The returned user carries the in-memory
// attributes Lucid serializes.
func Register(ctx context.Context, pool *pgxpool.Pool, email, password, fullname string) (dbgen.PublicUser, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return dbgen.PublicUser{}, err
	}
	return create(ctx, pool, email, &hash, fullname, nil)
}

func create(ctx context.Context, pool *pgxpool.Pool, email string, password *string, name string, inTx func(*dbgen.Queries) (*dbgen.PublicUser, error)) (dbgen.PublicUser, error) {
	var user dbgen.PublicUser
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if inTx != nil {
			existing, err := inTx(q)
			if err != nil || existing != nil {
				if existing != nil {
					user = *existing
				}
				return err
			}
		}
		created := Now()
		id, err := q.CreatePublicUser(ctx, dbgen.CreatePublicUserParams{Email: &email, Password: password, AccountStatus: "active", CreatedAt: Timestamp(created), UpdatedAt: Timestamp(created)})
		if err != nil {
			return err
		}
		memberID, updated := MemberID(id), Now()
		if err := q.SetMemberID(ctx, dbgen.SetMemberIDParams{ID: id, MemberID: &memberID, UpdatedAt: Timestamp(updated)}); err != nil {
			return err
		}
		profileTime := Now()
		if err := q.CreateProfile(ctx, dbgen.CreateProfileParams{UserID: &id, Name: name, CreatedAt: Timestamp(profileTime), UpdatedAt: Timestamp(profileTime)}); err != nil {
			return err
		}
		user = dbgen.PublicUser{ID: id, Email: &email, Password: password, CreatedAt: Timestamp(created), UpdatedAt: Timestamp(updated), MemberID: &memberID, AccountStatus: "active"}
		return nil
	})
	return user, err
}

// ResolveGoogleAccount is google_account_service.ts: under an advisory lock
// on the email, reuse the single case-insensitive match (active or the
// migrated no_account default) or create a password-less account. created
// reports which serialization order applies.
func ResolveGoogleAccount(ctx context.Context, pool *pgxpool.Pool, identity google.Identity) (user dbgen.PublicUser, created bool, err error) {
	user, err = create(ctx, pool, identity.Email, nil, identity.Name, func(q *dbgen.Queries) (*dbgen.PublicUser, error) {
		if err := q.LockEmail(ctx, identity.Email); err != nil {
			return nil, err
		}
		users, err := q.PublicUsersByLowerEmail(ctx, identity.Email)
		if err != nil {
			return nil, err
		}
		if len(users) > 1 {
			return nil, &google.LoginError{Code: "GOOGLE_EMAIL_PASSWORD_REQUIRED"}
		}
		if len(users) == 1 {
			if users[0].AccountStatus != "active" && users[0].AccountStatus != "no_account" {
				return nil, &google.LoginError{Code: "GOOGLE_ACCOUNT_INACTIVE"}
			}
			return &users[0], nil
		}
		created = true
		return nil, nil
	})
	return user, created, err
}

// ProfileFor is Profile.findBy('user_id', id); nil when the member has none.
func ProfileFor(ctx context.Context, q *dbgen.Queries, id int32) (*dbgen.Profile, error) {
	profile, err := q.ProfileByUserID(ctx, &id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// UserByEmail is PublicUser.findBy('email', email); nil when absent.
func UserByEmail(ctx context.Context, q *dbgen.Queries, email string) (*dbgen.PublicUser, error) {
	user, err := q.PublicUserByEmail(ctx, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
