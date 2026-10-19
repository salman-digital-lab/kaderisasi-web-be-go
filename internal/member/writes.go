package member

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
)

var achievementColumns = map[string]string{"name": "varchar", "description": "text", "achievement_date": "date", "type": "integer",
	"proof": "varchar", "status": "integer", "updated_at": "timestamptz"}

// SaveAchievement merges changes into the stored achievement like
// achievement.merge(updateData).save(): only differing values are written,
// with updated_at, in attribute order.
func SaveAchievement(ctx context.Context, db dbgen.DBTX, row dbgen.Achievement, current, changes Object, zone *time.Location) (Object, error) {
	original := current.Value()
	dirty := map[string]string{}
	for _, c := range changes {
		raw, _ := marshal(c.Value)
		next, _ := js.Parse(raw)
		old, _ := original.Get(c.Key)
		if !valueEqual(old, next) {
			dirty[c.Key] = prepareText(next)
		}
		current = current.Set(c.Key, c.Value)
	}
	if len(dirty) == 0 {
		return current, nil
	}
	updated := Now()
	current = current.Set("updated_at", ISO(updated, zone))
	dirty["updated_at"] = updated.In(zone).Format("2006-01-02T15:04:05.000-07:00")
	return current, update(ctx, db, "achievements", achievementColumns, current, dirty, row.ID)
}

func prepareText(v js.Value) string {
	switch v.Kind {
	case 's':
		return v.String
	case 'f':
		return js.ToString(v.Number)
	case 'b':
		return strconv.FormatBool(v.Bool)
	}
	raw, _ := v.MarshalJSON()
	return string(raw)
}

// update runs a knex-shaped UPDATE of the dirty attributes, in the model's
// attribute order, and reports database errors with knex's message.
func update(ctx context.Context, db dbgen.DBTX, table string, types map[string]string, attributes Object, dirty map[string]string, id int32) error {
	var sets, knexSets []string
	var args []any
	for _, p := range attributes {
		v, ok := dirty[p.Key]
		if !ok {
			continue
		}
		if v == "\x00null" {
			args = append(args, nil)
		} else {
			args = append(args, v)
		}
		n := strconv.Itoa(len(args))
		sets = append(sets, `"`+p.Key+`" = CAST($`+n+`::text AS `+types[p.Key]+`)`)
		knexSets = append(knexSets, `"`+p.Key+`" = $`+n)
	}
	args = append(args, id)
	where := `"id" = $` + strconv.Itoa(len(args))
	_, err := db.Exec(ctx, `UPDATE `+table+` SET `+strings.Join(sets, ", ")+` WHERE `+where, args...)
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return errors.New(`update "` + table + `" set ` + strings.Join(knexSets, ", ") + ` where ` + where + ` - ` + pg.Message)
	}
	return err
}

// SubmitMember is MembersController.submit's transaction: a member without
// an account (no email or password), its member id, and its profile.
func SubmitMember(ctx context.Context, pool *pgxpool.Pool, payload js.Value) error {
	str := func(k string) *string {
		if v, ok := payload.Get(k); ok && v.Kind == 's' {
			return &v.String
		}
		return nil
	}
	num := func(k string) *string {
		if v, ok := payload.Get(k); ok && v.Kind == 'f' {
			s := js.ToString(v.Number)
			return &s
		}
		return nil
	}
	jsonOf := func(v js.Value) string { raw, _ := v.MarshalJSON(); return string(raw) }
	extra, hasExtra := payload.Get("extra_data")
	path := js.Value{}
	if hasExtra {
		path, _ = extra.Get("kaderisasi_path")
	}
	_, badges := Kaderisasi(path)
	education, _ := payload.Get("education_history")
	educationJSON := string(mustJSON(EducationHistory([]byte(jsonOf(education)))))
	if _, ok := payload.Get("education_history"); !ok {
		educationJSON = "[]"
	}
	work := "[]"
	if v, ok := payload.Get("work_history"); ok {
		work = jsonOf(v)
	}
	extraJSON := "{}"
	if hasExtra {
		extraJSON = jsonOf(extra)
	}
	var birth *string
	if v := str("birth_date"); v != nil && *v != "" {
		formatted := invalidJSDate
		if at, ok := js.ParseDateString(*v); ok {
			formatted = at.In(js.Local).Format("2006-01-02T15:04:05.000-07:00")
		}
		birth = &formatted
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		created := Now()
		id, err := q.CreatePublicUser(ctx, dbgen.CreatePublicUserParams{AccountStatus: "no_account", CreatedAt: Timestamp(created), UpdatedAt: Timestamp(created)})
		if err != nil {
			return InsertError("public_users", map[string]bool{"account_status": true, "created_at": true, "email": true, "password": true, "updated_at": true}, err)
		}
		memberID := MemberID(id)
		if err := q.SetMemberID(ctx, dbgen.SetMemberIDParams{ID: id, MemberID: &memberID, UpdatedAt: Timestamp(Now())}); err != nil {
			return UpdateError("public_users", []string{"member_id", "updated_at"}, err)
		}
		badgesJSON := string(mustJSON(badges))
		name := ""
		if v := str("name"); v != nil {
			name = *v
		}
		profileTime := Now()
		_, err = tx.Exec(ctx, `INSERT INTO profiles (user_id, name, badges, gender, personal_id, whatsapp, line, instagram, tiktok, linkedin,
			province_id, city_id, country, birth_date, origin_province_id, origin_city_id, education_history, work_history, extra_data, created_at, updated_at)
			VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, $8, $9, $10, CAST($11::text AS integer), CAST($12::text AS integer), $13, CAST($14::text AS date),
			CAST($15::text AS integer), CAST($16::text AS integer), $17::jsonb, $18::jsonb, $19::jsonb, $20, $20)`,
			id, name, badgesJSON, str("gender"), str("personal_id"), str("whatsapp"), str("line"), str("instagram"), str("tiktok"), str("linkedin"),
			num("province_id"), num("city_id"), str("country"), birth, num("origin_province_id"), num("origin_city_id"), educationJSON, work, extraJSON,
			Timestamp(profileTime))
		if err != nil {
			defined := func(v *string) bool { return v != nil }
			return InsertError("profiles", map[string]bool{"user_id": true, "name": true, "badges": true, "gender": defined(str("gender")),
				"personal_id": defined(str("personal_id")), "whatsapp": defined(str("whatsapp")), "line": defined(str("line")),
				"instagram": defined(str("instagram")), "tiktok": defined(str("tiktok")), "linkedin": defined(str("linkedin")),
				"province_id": defined(num("province_id")), "city_id": defined(num("city_id")), "country": defined(str("country")),
				"birth_date": birth != nil, "origin_province_id": defined(num("origin_province_id")), "origin_city_id": defined(num("origin_city_id")),
				"education_history": true, "work_history": true, "extra_data": true, "created_at": true, "updated_at": true}, err)
		}
		return nil
	})
}
