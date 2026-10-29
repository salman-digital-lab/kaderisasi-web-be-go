package perintis

// JSON shapes of the /v2/perintis endpoints. They follow the
// kaderisasi-perintis-fe contract (src/api/types.ts): camelCase names, null
// for absent values, months as YYYY-MM-DD, instants as RFC 3339.

type Profile struct {
	UserID     int32   `json:"userId"`
	Name       string  `json:"name"`
	Email      string  `json:"email"`
	Gender     string  `json:"gender"`
	Phone      string  `json:"phone"`
	Instagram  *string `json:"instagram"`
	Linkedin   *string `json:"linkedin"`
	University string  `json:"university"`
	Major      string  `json:"major"`
	IntakeYear int32   `json:"intakeYear"`
	Domicile   string  `json:"domicile"`
	Level      string  `json:"level"`
	SSCStatus  string  `json:"sscStatus"`
	LMDStatus  string  `json:"lmdStatus"`
}

type Participant struct {
	UserID                 int32   `json:"userId"`
	IsMentee               bool    `json:"isMentee"`
	IsMentor               bool    `json:"isMentor"`
	IsCoordinator          bool    `json:"isCoordinator"`
	Nim                    *string `json:"nim"`
	KipK                   *bool   `json:"kipK"`
	Category               *string `json:"category"`
	KaderisasiOverride     *string `json:"kaderisasiOverride"`
	KaderisasiOverrideNote *string `json:"kaderisasiOverrideNote"`
	MentorUserID           *int32  `json:"mentorUserId"`
	CoordinatorUserID      *int32  `json:"coordinatorUserId"`
	Status                 string  `json:"status"`
	InactiveReason         *string `json:"inactiveReason"`
	HasAccount             bool    `json:"hasAccount"`
}

type PersonSummary struct {
	UserID int32   `json:"userId"`
	Name   string  `json:"name"`
	Phone  *string `json:"phone"`
}

type ActivityItem struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Date        string `json:"date"`
	Status      string `json:"status"`
	Certificate bool   `json:"certificate"`
}

type ClubItem struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

type Achievement struct {
	Name   string `json:"name"`
	Level  string `json:"level"`
	Year   int    `json:"year"`
	Status string `json:"status"`
}

type Markers struct {
	LapbulPending      bool `json:"lapbulPending"`
	ResponseUnread     bool `json:"responseUnread"`
	JournalsToRespond  int  `json:"journalsToRespond"`
	OpenFlags          int  `json:"openFlags"`
	ReportPending      bool `json:"reportPending"`
	ReportReviewUnread bool `json:"reportReviewUnread"`
	ReportsToVerify    int  `json:"reportsToVerify"`
}

type Me struct {
	Profile     Profile        `json:"profile"`
	Participant Participant    `json:"participant"`
	Roles       []string       `json:"roles"`
	Mentor      *PersonSummary `json:"mentor"`
	Coordinator *PersonSummary `json:"coordinator"`
	Markers     Markers        `json:"markers"`
}

type ProfileSummary struct {
	Month         *string `json:"month"`
	JournalStatus *string `json:"journalStatus"`
	LastResponse  *string `json:"lastResponse"`
	SessionDate   *string `json:"sessionDate"`
}

type FullProfile struct {
	Profile         Profile        `json:"profile"`
	Participant     Participant    `json:"participant"`
	Tingkat         int            `json:"tingkat"`
	Mentor          *PersonSummary `json:"mentor"`
	Coordinator     *PersonSummary `json:"coordinator"`
	Activities      []ActivityItem `json:"activities"`
	Clubs           []ClubItem     `json:"clubs"`
	Achievements    []Achievement  `json:"achievements"`
	SensitiveHidden bool           `json:"sensitiveHidden"`
	Summary         ProfileSummary `json:"summary"`
}

// File is an uploaded object kept in a jsonb list. Field is set for lapbul
// files (s3b, s7khs). PreviewURL stays null: files are served through the
// authorised /perintis/files endpoint.
type File struct {
	Field      string  `json:"field,omitempty"`
	Key        string  `json:"key"`
	Name       string  `json:"name"`
	Mime       string  `json:"mime"`
	Size       int64   `json:"size"`
	PreviewURL *string `json:"previewUrl"`
}

type JournalResponse struct {
	Appreciation    string  `json:"appreciation"`
	Text            string  `json:"text"`
	RevisionRequest *string `json:"revisionRequest"`
	FlagReason      *string `json:"flagReason"`
	RespondedBy     string  `json:"respondedBy"`
	RespondedAt     string  `json:"respondedAt"`
	ReadAt          *string `json:"readAt"`
}

type JournalFlag struct {
	Trigger     string  `json:"trigger"`
	Status      string  `json:"status"`
	DueAt       string  `json:"dueAt"`
	HandledBy   *string `json:"handledBy"`
	HandledAt   *string `json:"handledAt"`
	PrivateNote *string `json:"privateNote"`
}

type Followup struct {
	By   string `json:"by"`
	At   string `json:"at"`
	Note string `json:"note"`
}

type Journal struct {
	ID                 int32            `json:"id"`
	MenteeUserID       int32            `json:"menteeUserId"`
	Month              string           `json:"month"`
	MentorUserID       *int32           `json:"mentorUserId"`
	Category           string           `json:"category"`
	FormVersion        int32            `json:"formVersion"`
	Status             string           `json:"status"`
	Answers            Answers          `json:"answers"`
	Files              []File           `json:"files"`
	Pulse              Pulse            `json:"pulse"`
	WantsContact       bool             `json:"wantsContact"`
	HonestyConfirmedAt *string          `json:"honestyConfirmedAt"`
	SubmittedAt        *string          `json:"submittedAt"`
	IsLate             bool             `json:"isLate"`
	Revision           int32            `json:"revision"`
	ReopenReason       *string          `json:"reopenReason"`
	Response           *JournalResponse `json:"response"`
	Flag               *JournalFlag     `json:"flag"`
	Followup           *Followup        `json:"followup"`
	UpdatedAt          string           `json:"updatedAt"`
}

type JournalListItem struct {
	JournalID      *int32  `json:"journalId"`
	Month          string  `json:"month"`
	Status         string  `json:"status"`
	IsLate         bool    `json:"isLate"`
	SubmittedAt    *string `json:"submittedAt"`
	ResponseUnread bool    `json:"responseUnread"`
	Window         string  `json:"window"`
}

type SessionInfo struct {
	Date  string `json:"date"`
	Theme string `json:"theme"`
}

type AutoData struct {
	Name            string       `json:"name"`
	Nim             *string      `json:"nim"`
	University      string       `json:"university"`
	Major           string       `json:"major"`
	IntakeYear      int32        `json:"intakeYear"`
	Tingkat         int          `json:"tingkat"`
	Category        string       `json:"category"`
	MentorName      *string      `json:"mentorName"`
	CoordinatorName *string      `json:"coordinatorName"`
	Session         *SessionInfo `json:"session"`
	Activities      []string     `json:"activities"`
	SSCStatus       string       `json:"sscStatus"`
	LMDStatus       string       `json:"lmdStatus"`
	Clubs           []string     `json:"clubs"`
}

type JournalDetail struct {
	Month           string   `json:"month"`
	Category        string   `json:"category"`
	Window          string   `json:"window"`
	Journal         *Journal `json:"journal"`
	AutoData        AutoData `json:"autoData"`
	PreviousTargets []string `json:"previousTargets"`
	CanEdit         bool     `json:"canEdit"`
	LockedReason    *string  `json:"lockedReason"`
	RevisionDueAt   *string  `json:"revisionDueAt"`
}

type DashboardMentee struct {
	PersonSummary
	Category string `json:"category"`
	Tingkat  int    `json:"tingkat"`
}

type DashboardFlag struct {
	JournalFlag
	JournalID int32 `json:"journalId"`
}

type DashboardRow struct {
	Mentee         DashboardMentee `json:"mentee"`
	JournalID      *int32          `json:"journalId"`
	Status         string          `json:"status"`
	IsLate         bool            `json:"isLate"`
	SubmittedAt    *string         `json:"submittedAt"`
	Flag           *DashboardFlag  `json:"flag"`
	InactiveStreak bool            `json:"inactiveStreak"`
	// HeldBy names the mentor who answered this month's lapbul before the
	// mentee moved here; that lapbul stays with them (PRD §8.7).
	HeldBy *string `json:"heldBy"`
	// FormerMentee marks a mentee who moved to another mentor but whose
	// lapbul for this month this mentor answered.
	FormerMentee bool `json:"formerMentee"`
}

type MentorDashboard struct {
	Month     string         `json:"month"`
	Tingkat   int            `json:"tingkat"`
	Rows      []DashboardRow `json:"rows"`
	ToRespond int            `json:"toRespond"`
}

type HistoryItem struct {
	Month        string  `json:"month"`
	Status       string  `json:"status"`
	Pulse        Pulse   `json:"pulse"`
	Appreciation *string `json:"appreciation"`
	ResponseText *string `json:"responseText"`
}

type ReaderMentee struct {
	PersonSummary
	Category   string `json:"category"`
	Tingkat    int    `json:"tingkat"`
	University string `json:"university"`
	Major      string `json:"major"`
	KipK       *bool  `json:"kipK"`
}

type JournalForMentor struct {
	Journal             Journal       `json:"journal"`
	Mentee              ReaderMentee  `json:"mentee"`
	PreviousTargets     []string      `json:"previousTargets"`
	History             []HistoryItem `json:"history"`
	CanRespond          bool          `json:"canRespond"`
	RespondLockedReason *string       `json:"respondLockedReason"`
	// MovedAway is set for a mentor whose mentee has since moved to another
	// group: the lapbul stays readable here, the mentee's profile does not.
	MovedAway bool `json:"movedAway"`
}

type ReportReview struct {
	Appreciation string  `json:"appreciation"`
	Note         string  `json:"note"`
	ReviewerKind string  `json:"reviewerKind"`
	ReviewedBy   string  `json:"reviewedBy"`
	ReviewedAt   string  `json:"reviewedAt"`
	ReadAt       *string `json:"readAt"`
}

type SessionReport struct {
	ID              int32         `json:"id"`
	MentorUserID    int32         `json:"mentorUserId"`
	Month           string        `json:"month"`
	SessionDate     *string       `json:"sessionDate"`
	Mode            string        `json:"mode"`
	LocationOrLink  string        `json:"locationOrLink"`
	CurriculumCode  string        `json:"curriculumCode"`
	DeviationReason *string       `json:"deviationReason"`
	Summary         string        `json:"summary"`
	Photos          []File        `json:"photos"`
	CaseNote        *string       `json:"caseNote"`
	HasCase         bool          `json:"hasCase"`
	CaseStatus      *string       `json:"caseStatus"`
	Status          string        `json:"status"`
	SubmittedAt     *string       `json:"submittedAt"`
	IsLate          bool          `json:"isLate"`
	Review          *ReportReview `json:"review"`
	VerifiedAt      *string       `json:"verifiedAt"`
}

type MentorTasks struct {
	T1 bool `json:"t1"`
	T2 bool `json:"t2"`
	T3 bool `json:"t3"`
}

type ReportDetail struct {
	Month               string         `json:"month"`
	Mentor              PersonSummary  `json:"mentor"`
	Coordinator         *PersonSummary `json:"coordinator"`
	Report              *SessionReport `json:"report"`
	Curriculum          CurriculumItem `json:"curriculum"`
	Tasks               MentorTasks    `json:"tasks"`
	CanEdit             bool           `json:"canEdit"`
	UnrespondedJournals int            `json:"unrespondedJournals"`
	CanUnverify         bool           `json:"canUnverify"`
}

type ReportListItem struct {
	Month        string  `json:"month"`
	Status       string  `json:"status"`
	IsLate       bool    `json:"isLate"`
	VerifiedAt   *string `json:"verifiedAt"`
	ReviewUnread bool    `json:"reviewUnread"`
}

type CoordinatorReport struct {
	ID          int32   `json:"id"`
	Status      string  `json:"status"`
	IsLate      bool    `json:"isLate"`
	SubmittedAt *string `json:"submittedAt"`
	VerifiedAt  *string `json:"verifiedAt"`
	HasCase     bool    `json:"hasCase"`
}

type JournalCounts struct {
	Submitted int `json:"submitted"`
	Responded int `json:"responded"`
	Late      int `json:"late"`
}

type CoordinatorRow struct {
	Mentor         PersonSummary      `json:"mentor"`
	MenteeCount    int                `json:"menteeCount"`
	Tingkat        int                `json:"tingkat"`
	Report         *CoordinatorReport `json:"report"`
	Journals       JournalCounts      `json:"journals"`
	EscalatedFlags int                `json:"escalatedFlags"`
	PatternFlags   int                `json:"patternFlags"`
	Tasks          MentorTasks        `json:"tasks"`
}

type CoordinatorOverview struct {
	Month string           `json:"month"`
	Rows  []CoordinatorRow `json:"rows"`
}

// ---- pengelola ----------------------------------------------------------------

type Indicator struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Numerator   int     `json:"numerator"`
	Denominator int     `json:"denominator"`
	Target      float64 `json:"target"`
}

type CoordinatorRecap struct {
	Coordinator      PersonSummary `json:"coordinator"`
	Mentors          int           `json:"mentors"`
	Mentees          int           `json:"mentees"`
	Submitted        int           `json:"submitted"`
	Late             int           `json:"late"`
	Responded        int           `json:"responded"`
	ReportsSubmitted int           `json:"reportsSubmitted"`
	ReportsVerified  int           `json:"reportsVerified"`
	ActiveFlags      int           `json:"activeFlags"`
}

type StaffDashboard struct {
	Month         string             `json:"month"`
	Indicators    []Indicator        `json:"indicators"`
	ByCoordinator []CoordinatorRecap `json:"byCoordinator"`
	FollowupsOpen int                `json:"followupsOpen"`
	WithoutMentor int                `json:"withoutMentor"`
}

type FollowupItem struct {
	Source          string    `json:"source"`
	ID              int32     `json:"id"`
	Month           string    `json:"month"`
	Subject         string    `json:"subject"`
	MentorName      *string   `json:"mentorName"`
	CoordinatorName *string   `json:"coordinatorName"`
	Detail          string    `json:"detail"`
	OpenedAt        string    `json:"openedAt"`
	Status          string    `json:"status"`
	Handled         *Followup `json:"handled"`
}

type ParticipantRow struct {
	UserID          int32    `json:"userId"`
	Name            string   `json:"name"`
	Email           string   `json:"email"`
	Nim             *string  `json:"nim"`
	Roles           []string `json:"roles"`
	Category        *string  `json:"category"`
	KipK            *bool    `json:"kipK"`
	University      string   `json:"university"`
	Tingkat         int      `json:"tingkat"`
	MentorName      *string  `json:"mentorName"`
	CoordinatorName *string  `json:"coordinatorName"`
	MenteeCount     int      `json:"menteeCount"`
	Status          string   `json:"status"`
	HasAccount      bool     `json:"hasAccount"`
	ClubOK          bool     `json:"clubOk"`
}

type ParticipantDetail struct {
	Row         ParticipantRow    `json:"row"`
	Participant Participant       `json:"participant"`
	Profile     Profile           `json:"profile"`
	Mentees     []PersonSummary   `json:"mentees"`
	Mentors     []PersonSummary   `json:"mentors"`
	Journals    []JournalListItem `json:"journals"`
}

type ImportPreviewRow struct {
	Line    int      `json:"line"`
	Values  []string `json:"values"`
	Result  string   `json:"result"`
	Message string   `json:"message"`
}

type ImportPreview struct {
	Kind     string             `json:"kind"`
	Columns  []string           `json:"columns"`
	Rows     []ImportPreviewRow `json:"rows"`
	CanApply bool               `json:"canApply"`
}
