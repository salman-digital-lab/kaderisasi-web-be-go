-- GENERATED FROM ACE MIGRATIONS (copied from kaderisasi-admin-be-go/database/schema.sql). sqlc input only; never execute as a migration.
--
-- PostgreSQL database dump
--


-- Dumped from database version 17.7
-- Dumped by pg_dump version 18.3


--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--



--
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: -
--





--
-- Name: achievements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.achievements (
    id integer NOT NULL,
    user_id integer,
    name character varying(255),
    description text,
    achievement_date date,
    type integer,
    score integer,
    proof character varying(255),
    is_proof_deleted boolean DEFAULT false,
    status integer DEFAULT 0,
    remark text,
    approver_id integer,
    approved_at date,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: achievements_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.achievements_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: achievements_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.achievements_id_seq OWNED BY public.achievements.id;


--
-- Name: activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.activities (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    slug character varying(255) NOT NULL,
    description text,
    badge text,
    activity_start date,
    activity_end date,
    registration_start date,
    registration_end date,
    selection_start date,
    selection_end date,
    activity_type integer,
    activity_category integer,
    additional_config jsonb DEFAULT '{"images": [], "mandatory_profile_data": [], "custom_selection_status": [], "additional_questionnaire": []}'::jsonb,
    minimum_level integer DEFAULT 0,
    is_published boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    is_registration_open boolean DEFAULT true NOT NULL,
    club_id integer,
    certificate_template_id integer
);


--
-- Name: activities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.activities_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: activities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.activities_id_seq OWNED BY public.activities.id;


--
-- Name: activity_registrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.activity_registrations (
    id integer NOT NULL,
    user_id integer,
    activity_id integer,
    status character varying(50),
    questionnaire_answer jsonb DEFAULT '{}'::jsonb,
    scoring_data jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    guest_data jsonb
);

CREATE TABLE public.activity_scoring_rubrics (
    id serial PRIMARY KEY,
    activity_id integer NOT NULL UNIQUE REFERENCES public.activities(id) ON DELETE CASCADE,
    definition jsonb NOT NULL,
    revision integer NOT NULL DEFAULT 1,
    locked_at timestamptz,
    updated_by integer NOT NULL REFERENCES public.admin_users(id),
    updated_at timestamptz NOT NULL
);

CREATE TABLE public.activity_scoring_publications (
    id serial PRIMARY KEY,
    activity_id integer NOT NULL REFERENCES public.activities(id) ON DELETE RESTRICT,
    registration_id integer NOT NULL REFERENCES public.activity_registrations(id) ON DELETE RESTRICT,
    revision integer NOT NULL,
    action varchar(16) NOT NULL,
    snapshot jsonb,
    actor_id integer NOT NULL REFERENCES public.admin_users(id),
    created_at timestamptz NOT NULL,
    UNIQUE (registration_id, revision)
);


--
-- Name: activity_registrations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.activity_registrations_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: activity_registrations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.activity_registrations_id_seq OWNED BY public.activity_registrations.id;


--
-- Name: admin_auth_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_auth_identities (
    id integer NOT NULL,
    admin_user_id integer NOT NULL,
    provider character varying(40) NOT NULL,
    provider_subject character varying(255) NOT NULL,
    email character varying(255) NOT NULL,
    last_used_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone
);


--
-- Name: admin_auth_identities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_auth_identities_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_auth_identities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_auth_identities_id_seq OWNED BY public.admin_auth_identities.id;


--
-- Name: admin_refresh_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_refresh_tokens (
    id integer NOT NULL,
    family_id uuid NOT NULL,
    admin_user_id integer NOT NULL,
    token_hash character varying(64) NOT NULL,
    parent_token_id integer,
    replaced_by_token_id integer,
    user_agent character varying(500),
    ip_address character varying(100),
    expires_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    revocation_reason character varying(100),
    created_at timestamp with time zone NOT NULL
);


--
-- Name: admin_refresh_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_refresh_tokens_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_refresh_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_refresh_tokens_id_seq OWNED BY public.admin_refresh_tokens.id;


--
-- Name: admin_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_users (
    id integer NOT NULL,
    email character varying(255) NOT NULL,
    password character varying(255),
    display_name character varying(255),
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone,
    is_active boolean DEFAULT true NOT NULL,
    normalized_email character varying(255) NOT NULL,
    role_code character varying(100),
    additional_role_codes text[] DEFAULT '{}'::text[] NOT NULL
);


--
-- Name: admin_users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_users_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_users_id_seq OWNED BY public.admin_users.id;


--
-- Name: adonis_schema; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.adonis_schema (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    batch integer NOT NULL,
    migration_time timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: adonis_schema_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.adonis_schema_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: adonis_schema_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.adonis_schema_id_seq OWNED BY public.adonis_schema.id;


--
-- Name: adonis_schema_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.adonis_schema_versions (
    version integer NOT NULL
);


--
-- Name: certificate_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.certificate_templates (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    background_image character varying(255),
    template_data jsonb DEFAULT '{}'::jsonb,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    lifecycle_status character varying(20) DEFAULT 'draft'::character varying NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    background_asset_version integer DEFAULT 0 NOT NULL,
    published_at timestamp with time zone,
    archived_at timestamp with time zone,
    CONSTRAINT certificate_templates_lifecycle_status_check CHECK (((lifecycle_status)::text = ANY ((ARRAY['draft'::character varying, 'published'::character varying, 'archived'::character varying])::text[])))
);


--
-- Name: certificate_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.certificate_templates_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: certificate_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.certificate_templates_id_seq OWNED BY public.certificate_templates.id;


--
-- Name: cities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cities (
    id integer NOT NULL,
    province_id integer NOT NULL,
    name character varying(255),
    is_active boolean DEFAULT true
);


--
-- Name: cities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cities_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cities_id_seq OWNED BY public.cities.id;


--
-- Name: club_member_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.club_member_roles (
    id integer NOT NULL,
    club_registration_id integer NOT NULL,
    role_name character varying(150) NOT NULL,
    start_date date,
    end_date date,
    is_primary boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: club_member_roles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.club_member_roles_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: club_member_roles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.club_member_roles_id_seq OWNED BY public.club_member_roles.id;


--
-- Name: club_registrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.club_registrations (
    id integer NOT NULL,
    club_id integer,
    member_id integer,
    status character varying(50) DEFAULT 'PENDING'::character varying,
    additional_data jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: club_registrations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.club_registrations_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: club_registrations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.club_registrations_id_seq OWNED BY public.club_registrations.id;


--
-- Name: clubs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.clubs (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    short_description character varying(200),
    logo character varying(255),
    media jsonb DEFAULT '{"items": []}'::jsonb,
    start_period date,
    end_period date,
    is_show boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    registration_info jsonb DEFAULT '{"registration_info": ""}'::jsonb,
    is_registration_open boolean DEFAULT false,
    registration_end_date date,
    club_type character varying(50) DEFAULT 'UNIT'::character varying NOT NULL,
    CONSTRAINT clubs_club_type_check CHECK (((club_type)::text = ANY ((ARRAY['UNIT'::character varying, 'CLUB_KEPROFESIAN'::character varying, 'CLUB_BAHASA'::character varying, 'AVISMAN_REGIONAL'::character varying])::text[])))
);


--
-- Name: clubs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.clubs_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: clubs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.clubs_id_seq OWNED BY public.clubs.id;


--
-- Name: countries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.countries (
    id integer NOT NULL,
    name character varying(100) NOT NULL,
    code character varying(10) NOT NULL
);


--
-- Name: countries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.countries_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: countries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.countries_id_seq OWNED BY public.countries.id;


--
-- Name: custom_forms; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_forms (
    id integer NOT NULL,
    form_name character varying(255) NOT NULL,
    form_description text,
    feature_id integer,
    form_schema jsonb DEFAULT '{}'::jsonb,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    feature_type text,
    post_submission_info text,
    CONSTRAINT custom_forms_feature_type_check CHECK ((feature_type = ANY (ARRAY['activity_registration'::text, 'club_registration'::text, 'independent_form'::text])))
);


--
-- Name: custom_forms_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.custom_forms_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: custom_forms_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.custom_forms_id_seq OWNED BY public.custom_forms.id;


--
-- Name: issued_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.issued_certificates (
    approval_snapshot jsonb,
    id integer NOT NULL,
    certificate_code character varying(255) NOT NULL,
    registration_id integer NOT NULL,
    activity_id integer NOT NULL,
    user_id integer,
    template_id integer NOT NULL,
    template_snapshot jsonb NOT NULL,
    participant_snapshot jsonb NOT NULL,
    issued_by integer,
    issued_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    revoked_reason text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    activity_snapshot jsonb NOT NULL,
    snapshot_version integer DEFAULT 1 NOT NULL,
    template_version integer DEFAULT 1 NOT NULL,
    revoked_by integer
);


--
-- Name: issued_certificates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.issued_certificates_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: issued_certificates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.issued_certificates_id_seq OWNED BY public.issued_certificates.id;


--
-- Name: monthly_leaderboards; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.monthly_leaderboards (
    id integer NOT NULL,
    user_id integer,
    month date,
    score_academic integer,
    score_competition integer,
    score_organizational integer,
    score integer,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: monthly_leaderboards_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.monthly_leaderboards_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: monthly_leaderboards_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.monthly_leaderboards_id_seq OWNED BY public.monthly_leaderboards.id;


--
-- Name: profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.profiles (
    id integer NOT NULL,
    user_id integer,
    name character varying(255) NOT NULL,
    gender character varying(1),
    personal_id character varying(50),
    picture character varying(255),
    whatsapp character varying(35),
    line character varying(50),
    instagram character varying(50),
    tiktok character varying(50),
    linkedin character varying(255),
    province_id integer,
    city_id integer,
    university_id integer,
    major character varying(50),
    intake_year integer,
    level integer DEFAULT 0,
    badges jsonb DEFAULT '[]'::jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    birth_date date,
    origin_province_id integer,
    origin_city_id integer,
    country character varying(100),
    education_history jsonb DEFAULT '[]'::jsonb,
    work_history jsonb DEFAULT '[]'::jsonb,
    extra_data jsonb DEFAULT '{}'::jsonb
);


--
-- Name: profiles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.profiles_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: profiles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.profiles_id_seq OWNED BY public.profiles.id;


--
-- Name: provinces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.provinces (
    id integer NOT NULL,
    name character varying(100),
    is_active boolean DEFAULT true
);


--
-- Name: provinces_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.provinces_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: provinces_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.provinces_id_seq OWNED BY public.provinces.id;


--
-- Name: public_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.public_users (
    id integer NOT NULL,
    email character varying(255),
    password character varying(255),
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone,
    member_id character varying(255),
    account_status character varying(20) DEFAULT 'no_account'::character varying NOT NULL
);


--
-- Name: public_users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.public_users_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: public_users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.public_users_id_seq OWNED BY public.public_users.id;


--
-- Name: ruang_curhats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ruang_curhats (
    id integer NOT NULL,
    user_id integer,
    problem_ownership integer,
    owner_name character varying(50),
    problem_category character varying(50),
    problem_description text,
    handling_technic character varying(255),
    counselor_gender character varying(255),
    counselor_id integer,
    status integer DEFAULT 0,
    additional_notes text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: ruang_curhats_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ruang_curhats_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ruang_curhats_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ruang_curhats_id_seq OWNED BY public.ruang_curhats.id;


--
-- Name: tickets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tickets (
    id integer NOT NULL,
    number character varying(40) NOT NULL,
    status character varying(30) DEFAULT 'open'::character varying NOT NULL,
    resolution character varying(30),
    requester_admin_user_id integer NOT NULL,
    requested_role_code character varying(100) NOT NULL,
    reason text NOT NULL,
    rejection_reason text,
    resolved_by_admin_user_id integer,
    resolved_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone,
    CONSTRAINT tickets_role_request_status CHECK (((status)::text = ANY ((ARRAY['open'::character varying, 'resolved'::character varying, 'cancelled'::character varying])::text[])))
);


--
-- Name: tickets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tickets_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tickets_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tickets_id_seq OWNED BY public.tickets.id;


--
-- Name: universities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.universities (
    id integer NOT NULL,
    name character varying(100) NOT NULL,
    province_id integer,
    is_active boolean DEFAULT true
);


--
-- Name: universities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.universities_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: universities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.universities_id_seq OWNED BY public.universities.id;


--
-- Name: achievements id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.achievements ALTER COLUMN id SET DEFAULT nextval('public.achievements_id_seq'::regclass);


--
-- Name: activities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activities ALTER COLUMN id SET DEFAULT nextval('public.activities_id_seq'::regclass);


--
-- Name: activity_registrations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_registrations ALTER COLUMN id SET DEFAULT nextval('public.activity_registrations_id_seq'::regclass);


--
-- Name: admin_auth_identities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_auth_identities ALTER COLUMN id SET DEFAULT nextval('public.admin_auth_identities_id_seq'::regclass);


--
-- Name: admin_refresh_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_refresh_tokens ALTER COLUMN id SET DEFAULT nextval('public.admin_refresh_tokens_id_seq'::regclass);


--
-- Name: admin_users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_users ALTER COLUMN id SET DEFAULT nextval('public.admin_users_id_seq'::regclass);


--
-- Name: adonis_schema id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.adonis_schema ALTER COLUMN id SET DEFAULT nextval('public.adonis_schema_id_seq'::regclass);


--
-- Name: certificate_templates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.certificate_templates ALTER COLUMN id SET DEFAULT nextval('public.certificate_templates_id_seq'::regclass);


--
-- Name: cities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cities ALTER COLUMN id SET DEFAULT nextval('public.cities_id_seq'::regclass);


--
-- Name: club_member_roles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.club_member_roles ALTER COLUMN id SET DEFAULT nextval('public.club_member_roles_id_seq'::regclass);


--
-- Name: club_registrations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.club_registrations ALTER COLUMN id SET DEFAULT nextval('public.club_registrations_id_seq'::regclass);


--
-- Name: clubs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.clubs ALTER COLUMN id SET DEFAULT nextval('public.clubs_id_seq'::regclass);


--
-- Name: countries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.countries ALTER COLUMN id SET DEFAULT nextval('public.countries_id_seq'::regclass);


--
-- Name: custom_forms id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_forms ALTER COLUMN id SET DEFAULT nextval('public.custom_forms_id_seq'::regclass);


--
-- Name: issued_certificates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issued_certificates ALTER COLUMN id SET DEFAULT nextval('public.issued_certificates_id_seq'::regclass);


--
-- Name: monthly_leaderboards id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monthly_leaderboards ALTER COLUMN id SET DEFAULT nextval('public.monthly_leaderboards_id_seq'::regclass);


--
-- Name: profiles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profiles ALTER COLUMN id SET DEFAULT nextval('public.profiles_id_seq'::regclass);


--
-- Name: provinces id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provinces ALTER COLUMN id SET DEFAULT nextval('public.provinces_id_seq'::regclass);


--
-- Name: public_users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.public_users ALTER COLUMN id SET DEFAULT nextval('public.public_users_id_seq'::regclass);


--
-- Name: ruang_curhats id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ruang_curhats ALTER COLUMN id SET DEFAULT nextval('public.ruang_curhats_id_seq'::regclass);


--
-- Name: tickets id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets ALTER COLUMN id SET DEFAULT nextval('public.tickets_id_seq'::regclass);


--
-- Name: universities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.universities ALTER COLUMN id SET DEFAULT nextval('public.universities_id_seq'::regclass);


--
-- Name: achievements achievements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.achievements
    ADD CONSTRAINT achievements_pkey PRIMARY KEY (id);


--
-- Name: activities activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activities
    ADD CONSTRAINT activities_pkey PRIMARY KEY (id);


--
-- Name: activities activities_slug_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activities
    ADD CONSTRAINT activities_slug_unique UNIQUE (slug);


--
-- Name: activity_registrations activity_registrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_registrations
    ADD CONSTRAINT activity_registrations_pkey PRIMARY KEY (id);


--
-- Name: admin_auth_identities admin_auth_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_auth_identities
    ADD CONSTRAINT admin_auth_identities_pkey PRIMARY KEY (id);


--
-- Name: admin_auth_identities admin_auth_identities_provider_admin_user_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_auth_identities
    ADD CONSTRAINT admin_auth_identities_provider_admin_user_id_unique UNIQUE (provider, admin_user_id);


--
-- Name: admin_auth_identities admin_auth_identities_provider_provider_subject_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_auth_identities
    ADD CONSTRAINT admin_auth_identities_provider_provider_subject_unique UNIQUE (provider, provider_subject);


--
-- Name: admin_refresh_tokens admin_refresh_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_refresh_tokens
    ADD CONSTRAINT admin_refresh_tokens_pkey PRIMARY KEY (id);


--
-- Name: admin_refresh_tokens admin_refresh_tokens_token_hash_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_refresh_tokens
    ADD CONSTRAINT admin_refresh_tokens_token_hash_unique UNIQUE (token_hash);


--
-- Name: admin_users admin_users_email_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_users
    ADD CONSTRAINT admin_users_email_unique UNIQUE (email);


--
-- Name: admin_users admin_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_users
    ADD CONSTRAINT admin_users_pkey PRIMARY KEY (id);


--
-- Name: adonis_schema adonis_schema_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.adonis_schema
    ADD CONSTRAINT adonis_schema_pkey PRIMARY KEY (id);


--
-- Name: adonis_schema_versions adonis_schema_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.adonis_schema_versions
    ADD CONSTRAINT adonis_schema_versions_pkey PRIMARY KEY (version);


--
-- Name: certificate_templates certificate_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.certificate_templates
    ADD CONSTRAINT certificate_templates_pkey PRIMARY KEY (id);


--
-- Name: cities cities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cities
    ADD CONSTRAINT cities_pkey PRIMARY KEY (id);


--
-- Name: club_member_roles club_member_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.club_member_roles
    ADD CONSTRAINT club_member_roles_pkey PRIMARY KEY (id);


--
-- Name: club_registrations club_registrations_club_id_member_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.club_registrations
    ADD CONSTRAINT club_registrations_club_id_member_id_unique UNIQUE (club_id, member_id);


--
-- Name: club_registrations club_registrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.club_registrations
    ADD CONSTRAINT club_registrations_pkey PRIMARY KEY (id);


--
-- Name: clubs clubs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.clubs
    ADD CONSTRAINT clubs_pkey PRIMARY KEY (id);


--
-- Name: countries countries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.countries
    ADD CONSTRAINT countries_pkey PRIMARY KEY (id);


--
-- Name: custom_forms custom_forms_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_forms
    ADD CONSTRAINT custom_forms_pkey PRIMARY KEY (id);


--
-- Name: issued_certificates issued_certificates_certificate_code_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issued_certificates
    ADD CONSTRAINT issued_certificates_certificate_code_unique UNIQUE (certificate_code);


--
-- Name: issued_certificates issued_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issued_certificates
    ADD CONSTRAINT issued_certificates_pkey PRIMARY KEY (id);


--
-- Name: issued_certificates issued_certificates_registration_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

CREATE UNIQUE INDEX issued_certificates_active_registration_unique ON public.issued_certificates (registration_id) WHERE revoked_at IS NULL;


--
-- Name: monthly_leaderboards monthly_leaderboards_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monthly_leaderboards
    ADD CONSTRAINT monthly_leaderboards_pkey PRIMARY KEY (id);


--
-- Name: profiles profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profiles
    ADD CONSTRAINT profiles_pkey PRIMARY KEY (id);


--
-- Name: provinces provinces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provinces
    ADD CONSTRAINT provinces_pkey PRIMARY KEY (id);


--
-- Name: public_users public_users_email_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.public_users
    ADD CONSTRAINT public_users_email_unique UNIQUE (email);


--
-- Name: public_users public_users_member_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.public_users
    ADD CONSTRAINT public_users_member_id_unique UNIQUE (member_id);


--
-- Name: public_users public_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.public_users
    ADD CONSTRAINT public_users_pkey PRIMARY KEY (id);


--
-- Name: ruang_curhats ruang_curhats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ruang_curhats
    ADD CONSTRAINT ruang_curhats_pkey PRIMARY KEY (id);


--
-- Name: tickets tickets_number_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_number_unique UNIQUE (number);


--
-- Name: tickets tickets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_pkey PRIMARY KEY (id);


--
-- Name: universities universities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.universities
    ADD CONSTRAINT universities_pkey PRIMARY KEY (id);


--
-- Name: admin_refresh_tokens_admin_user_id_family_id_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_refresh_tokens_admin_user_id_family_id_index ON public.admin_refresh_tokens USING btree (admin_user_id, family_id);


--
-- Name: admin_refresh_tokens_expires_at_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_refresh_tokens_expires_at_index ON public.admin_refresh_tokens USING btree (expires_at);


--
-- Name: admin_users_normalized_email_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX admin_users_normalized_email_unique ON public.admin_users USING btree (normalized_email);


--
-- Name: admin_users_role_code_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_users_role_code_index ON public.admin_users USING btree (role_code);


--
-- Name: idx_achievements_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_achievements_status ON public.achievements USING btree (status);


--
-- Name: idx_achievements_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_achievements_type ON public.achievements USING btree (type);


--
-- Name: idx_achievements_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_achievements_user_id ON public.achievements USING btree (user_id);


--
-- Name: idx_achievements_user_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_achievements_user_status ON public.achievements USING btree (user_id, status);


--
-- Name: idx_act_reg_activity_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_act_reg_activity_status ON public.activity_registrations USING btree (activity_id, status);


--
-- Name: idx_act_reg_user_activity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_act_reg_user_activity ON public.activity_registrations USING btree (user_id, activity_id);


--
-- Name: idx_activities_certificate_template; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_certificate_template ON public.activities USING btree (certificate_template_id);


--
-- Name: idx_activities_club; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_club ON public.activities USING btree (club_id);


--
-- Name: idx_club_member_roles_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_club_member_roles_name ON public.club_member_roles USING btree (role_name);


--
-- Name: idx_club_member_roles_registration; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_club_member_roles_registration ON public.club_member_roles USING btree (club_registration_id);


--
-- Name: idx_club_reg_member; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_club_reg_member ON public.club_registrations USING btree (member_id);


--
-- Name: idx_club_reg_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_club_reg_status ON public.club_registrations USING btree (status);


--
-- Name: idx_clubs_club_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_clubs_club_type ON public.clubs USING btree (club_type);


--
-- Name: idx_custom_forms_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_custom_forms_active ON public.custom_forms USING btree (is_active);


--
-- Name: idx_custom_forms_feature; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_custom_forms_feature ON public.custom_forms USING btree (feature_type, feature_id);


--
-- Name: idx_issued_certificates_activity_issued_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_issued_certificates_activity_issued_at ON public.issued_certificates USING btree (activity_id, issued_at);


--
-- Name: idx_issued_certificates_user_issued_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_issued_certificates_user_issued_at ON public.issued_certificates USING btree (user_id, issued_at);


--
-- Name: idx_monthly_lb_month_score; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_monthly_lb_month_score ON public.monthly_leaderboards USING btree (month, score);


--
-- Name: idx_monthly_lb_user_month; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_monthly_lb_user_month ON public.monthly_leaderboards USING btree (user_id, month);


--
-- Name: idx_profiles_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_profiles_name ON public.profiles USING btree (name);


--
-- Name: idx_profiles_province; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_profiles_province ON public.profiles USING btree (province_id);


--
-- Name: idx_profiles_university; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_profiles_university ON public.profiles USING btree (university_id);


--
-- Name: idx_profiles_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_profiles_user_id ON public.profiles USING btree (user_id);


--
-- Name: issued_certificates_activity_id_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issued_certificates_activity_id_index ON public.issued_certificates USING btree (activity_id);


--
-- Name: issued_certificates_registration_id_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issued_certificates_registration_id_index ON public.issued_certificates USING btree (registration_id);


--
-- Name: issued_certificates_template_id_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issued_certificates_template_id_index ON public.issued_certificates USING btree (template_id);


--
-- Name: tickets_one_open_role_request; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX tickets_one_open_role_request ON public.tickets USING btree (requester_admin_user_id, requested_role_code) WHERE ((status)::text = 'open'::text);


--
-- Name: tickets_requested_role_code_status_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tickets_requested_role_code_status_index ON public.tickets USING btree (requested_role_code, status);


--
-- Name: tickets_requester_admin_user_id_status_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tickets_requester_admin_user_id_status_index ON public.tickets USING btree (requester_admin_user_id, status);


--
-- Name: universities_name_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX universities_name_index ON public.universities USING btree (name);


--
-- Name: achievements achievements_approver_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.achievements
    ADD CONSTRAINT achievements_approver_id_foreign FOREIGN KEY (approver_id) REFERENCES public.admin_users(id) ON DELETE CASCADE;


--
-- Name: achievements achievements_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.achievements
    ADD CONSTRAINT achievements_user_id_foreign FOREIGN KEY (user_id) REFERENCES public.public_users(id) ON DELETE CASCADE;


--
-- Name: activities activities_certificate_template_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activities
    ADD CONSTRAINT activities_certificate_template_id_foreign FOREIGN KEY (certificate_template_id) REFERENCES public.certificate_templates(id) ON DELETE RESTRICT;


--
-- Name: activities activities_club_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activities
    ADD CONSTRAINT activities_club_id_foreign FOREIGN KEY (club_id) REFERENCES public.clubs(id) ON DELETE SET NULL;


--
-- Name: activity_registrations activity_registrations_activity_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_registrations
    ADD CONSTRAINT activity_registrations_activity_id_foreign FOREIGN KEY (activity_id) REFERENCES public.activities(id) ON DELETE CASCADE;


--
-- Name: activity_registrations activity_registrations_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_registrations
    ADD CONSTRAINT activity_registrations_user_id_foreign FOREIGN KEY (user_id) REFERENCES public.public_users(id) ON DELETE CASCADE;


--
-- Name: admin_auth_identities admin_auth_identities_admin_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_auth_identities
    ADD CONSTRAINT admin_auth_identities_admin_user_id_foreign FOREIGN KEY (admin_user_id) REFERENCES public.admin_users(id) ON DELETE CASCADE;


--
-- Name: admin_refresh_tokens admin_refresh_tokens_admin_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_refresh_tokens
    ADD CONSTRAINT admin_refresh_tokens_admin_user_id_foreign FOREIGN KEY (admin_user_id) REFERENCES public.admin_users(id) ON DELETE CASCADE;


--
-- Name: admin_refresh_tokens admin_refresh_tokens_parent_token_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_refresh_tokens
    ADD CONSTRAINT admin_refresh_tokens_parent_token_id_foreign FOREIGN KEY (parent_token_id) REFERENCES public.admin_refresh_tokens(id) ON DELETE SET NULL;


--
-- Name: admin_refresh_tokens admin_refresh_tokens_replaced_by_token_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_refresh_tokens
    ADD CONSTRAINT admin_refresh_tokens_replaced_by_token_id_foreign FOREIGN KEY (replaced_by_token_id) REFERENCES public.admin_refresh_tokens(id) ON DELETE SET NULL;


--
-- Name: cities cities_province_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cities
    ADD CONSTRAINT cities_province_id_foreign FOREIGN KEY (province_id) REFERENCES public.provinces(id) ON DELETE CASCADE;


--
-- Name: club_member_roles club_member_roles_club_registration_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.club_member_roles
    ADD CONSTRAINT club_member_roles_club_registration_id_foreign FOREIGN KEY (club_registration_id) REFERENCES public.club_registrations(id) ON DELETE CASCADE;


--
-- Name: club_registrations club_registrations_club_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.club_registrations
    ADD CONSTRAINT club_registrations_club_id_foreign FOREIGN KEY (club_id) REFERENCES public.clubs(id) ON DELETE CASCADE;


--
-- Name: club_registrations club_registrations_member_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.club_registrations
    ADD CONSTRAINT club_registrations_member_id_foreign FOREIGN KEY (member_id) REFERENCES public.public_users(id) ON DELETE CASCADE;


--
-- Name: issued_certificates issued_certificates_activity_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issued_certificates
    ADD CONSTRAINT issued_certificates_activity_id_foreign FOREIGN KEY (activity_id) REFERENCES public.activities(id);


--
-- Name: issued_certificates issued_certificates_issued_by_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issued_certificates
    ADD CONSTRAINT issued_certificates_issued_by_foreign FOREIGN KEY (issued_by) REFERENCES public.admin_users(id);


--
-- Name: issued_certificates issued_certificates_registration_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issued_certificates
    ADD CONSTRAINT issued_certificates_registration_id_foreign FOREIGN KEY (registration_id) REFERENCES public.activity_registrations(id) ON DELETE RESTRICT;


--
-- Name: issued_certificates issued_certificates_revoked_by_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issued_certificates
    ADD CONSTRAINT issued_certificates_revoked_by_foreign FOREIGN KEY (revoked_by) REFERENCES public.admin_users(id) ON DELETE SET NULL;


--
-- Name: issued_certificates issued_certificates_template_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issued_certificates
    ADD CONSTRAINT issued_certificates_template_id_foreign FOREIGN KEY (template_id) REFERENCES public.certificate_templates(id);


--
-- Name: issued_certificates issued_certificates_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issued_certificates
    ADD CONSTRAINT issued_certificates_user_id_foreign FOREIGN KEY (user_id) REFERENCES public.public_users(id);


--
-- Name: monthly_leaderboards monthly_leaderboards_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monthly_leaderboards
    ADD CONSTRAINT monthly_leaderboards_user_id_foreign FOREIGN KEY (user_id) REFERENCES public.public_users(id) ON DELETE CASCADE;


--
-- Name: profiles profiles_city_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profiles
    ADD CONSTRAINT profiles_city_id_foreign FOREIGN KEY (city_id) REFERENCES public.cities(id);


--
-- Name: profiles profiles_origin_city_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profiles
    ADD CONSTRAINT profiles_origin_city_id_foreign FOREIGN KEY (origin_city_id) REFERENCES public.cities(id);


--
-- Name: profiles profiles_origin_province_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profiles
    ADD CONSTRAINT profiles_origin_province_id_foreign FOREIGN KEY (origin_province_id) REFERENCES public.provinces(id);


--
-- Name: profiles profiles_province_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profiles
    ADD CONSTRAINT profiles_province_id_foreign FOREIGN KEY (province_id) REFERENCES public.provinces(id);


--
-- Name: profiles profiles_university_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profiles
    ADD CONSTRAINT profiles_university_id_foreign FOREIGN KEY (university_id) REFERENCES public.universities(id);


--
-- Name: profiles profiles_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profiles
    ADD CONSTRAINT profiles_user_id_foreign FOREIGN KEY (user_id) REFERENCES public.public_users(id) ON DELETE CASCADE;


--
-- Name: ruang_curhats ruang_curhats_counselor_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ruang_curhats
    ADD CONSTRAINT ruang_curhats_counselor_id_foreign FOREIGN KEY (counselor_id) REFERENCES public.admin_users(id) ON DELETE CASCADE;


--
-- Name: ruang_curhats ruang_curhats_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ruang_curhats
    ADD CONSTRAINT ruang_curhats_user_id_foreign FOREIGN KEY (user_id) REFERENCES public.public_users(id) ON DELETE CASCADE;


--
-- Name: tickets tickets_requester_admin_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_requester_admin_user_id_foreign FOREIGN KEY (requester_admin_user_id) REFERENCES public.admin_users(id) ON DELETE RESTRICT;


--
-- Name: tickets tickets_resolved_by_admin_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_resolved_by_admin_user_id_foreign FOREIGN KEY (resolved_by_admin_user_id) REFERENCES public.admin_users(id) ON DELETE SET NULL;


--
-- Name: universities universities_province_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.universities
    ADD CONSTRAINT universities_province_id_foreign FOREIGN KEY (province_id) REFERENCES public.provinces(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

CREATE TABLE public.courses (
    id serial PRIMARY KEY,
    title varchar(255) NOT NULL,
    summary text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    minimum_level integer NOT NULL DEFAULT 0 CHECK (minimum_level IN (0,3,6,10)),
    status varchar(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.course_lessons (
    id serial PRIMARY KEY,
    course_id integer NOT NULL REFERENCES public.courses(id) ON DELETE CASCADE,
    title varchar(255) NOT NULL,
    description text NOT NULL DEFAULT '',
    youtube_video_id varchar(11) NOT NULL DEFAULT '',
    position integer NOT NULL CHECK (position > 0),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.course_documents (
    id serial PRIMARY KEY,
    lesson_id integer NOT NULL REFERENCES public.course_lessons(id) ON DELETE CASCADE,
    storage_key varchar(255) NOT NULL UNIQUE,
    filename varchar(255) NOT NULL,
    size_bytes integer NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 20971520),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.course_lesson_progress (
    id serial PRIMARY KEY,
    user_id integer NOT NULL REFERENCES public.public_users(id) ON DELETE CASCADE,
    lesson_id integer NOT NULL REFERENCES public.course_lessons(id) ON DELETE CASCADE,
    first_visited_at timestamptz NOT NULL DEFAULT now(),
    last_visited_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (user_id,lesson_id)
);

CREATE TABLE public.certificate_approvals (
 id serial PRIMARY KEY,
 registration_id integer NOT NULL REFERENCES public.activity_registrations(id),
 activity_id integer NOT NULL REFERENCES public.activities(id),
 signer_id integer NOT NULL REFERENCES public.admin_users(id),
 requested_by integer NOT NULL REFERENCES public.admin_users(id),
 signer_name varchar(255) NOT NULL,
 signer_title varchar(120) NOT NULL,
 snapshot jsonb NOT NULL,
 content_hash varchar(64) NOT NULL,
 status varchar(20) NOT NULL DEFAULT 'pending',
 decided_by integer REFERENCES public.admin_users(id),
 decided_at timestamptz,
 reason varchar(500),
 certificate_id integer REFERENCES public.issued_certificates(id),
 created_at timestamptz NOT NULL,
 updated_at timestamptz,
 CONSTRAINT certificate_approvals_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
 CONSTRAINT certificate_approvals_decision_check CHECK ((status = 'pending' AND decided_at IS NULL AND decided_by IS NULL AND certificate_id IS NULL) OR (status = 'approved' AND decided_at IS NOT NULL AND decided_by = signer_id AND certificate_id IS NOT NULL) OR (status IN ('rejected', 'cancelled') AND decided_at IS NOT NULL AND decided_by IS NOT NULL AND certificate_id IS NULL))
);
CREATE UNIQUE INDEX certificate_approvals_pending_registration ON public.certificate_approvals(registration_id) WHERE status = 'pending';
CREATE INDEX certificate_approvals_signer_id_status_id_index ON public.certificate_approvals(signer_id, status, id);
CREATE INDEX certificate_approvals_activity_id_status_id_index ON public.certificate_approvals(activity_id, status, id);

CREATE TABLE urls (
    id character varying(10) PRIMARY KEY,
    original_url text NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    visit_count bigint NOT NULL DEFAULT 0
);

CREATE TABLE activity_courses (
 id serial PRIMARY KEY,
 activity_id integer NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
 course_id integer NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
 position integer NOT NULL CHECK (position > 0),
 UNIQUE (activity_id, course_id)
);
CREATE INDEX activity_courses_course_id_index ON activity_courses(course_id);
CREATE VIEW activity_course_progress AS
SELECT ar.activity_id, ar.id AS registration_id, ac.course_id, ac.position,
 totals.total_lessons, progress.completed_lessons,
 CASE WHEN ar.user_id IS NULL THEN 'unverifiable'
 WHEN totals.total_lessons = 0 THEN 'empty'
 WHEN progress.completed_lessons = totals.total_lessons THEN 'completed'
 WHEN progress.visited_lessons > 0 THEN 'in_progress'
 ELSE 'not_started' END AS status
FROM activity_registrations ar
JOIN activity_courses ac ON ac.activity_id = ar.activity_id
CROSS JOIN LATERAL (
 SELECT count(*)::integer AS total_lessons FROM course_lessons l
 WHERE l.course_id = ac.course_id AND l.deleted_at IS NULL
) totals
CROSS JOIN LATERAL (
 SELECT count(*)::integer AS visited_lessons,
 count(*) FILTER (WHERE p.completed_at IS NOT NULL)::integer AS completed_lessons
 FROM course_lesson_progress p JOIN course_lessons l ON l.id = p.lesson_id
 WHERE p.user_id = ar.user_id AND l.course_id = ac.course_id AND l.deleted_at IS NULL
) progress;

CREATE TABLE club_courses (
 id serial PRIMARY KEY,
 club_id integer NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
 course_id integer NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
 position integer NOT NULL CHECK (position > 0),
 UNIQUE (club_id, course_id)
);
CREATE INDEX club_courses_course_id_index ON club_courses(course_id);
CREATE VIEW club_course_progress AS
SELECT ar.club_id, ar.id AS registration_id, ac.course_id, ac.position,
 totals.total_lessons, progress.completed_lessons,
 CASE WHEN ar.member_id IS NULL THEN 'unverifiable'
 WHEN totals.total_lessons = 0 THEN 'empty'
 WHEN progress.completed_lessons = totals.total_lessons THEN 'completed'
 WHEN progress.visited_lessons > 0 THEN 'in_progress'
 ELSE 'not_started' END AS status
FROM club_registrations ar
JOIN club_courses ac ON ac.club_id = ar.club_id
CROSS JOIN LATERAL (
 SELECT count(*)::integer AS total_lessons FROM course_lessons l
 WHERE l.course_id = ac.course_id AND l.deleted_at IS NULL
) totals
CROSS JOIN LATERAL (
 SELECT count(*)::integer AS visited_lessons,
 count(*) FILTER (WHERE p.completed_at IS NOT NULL)::integer AS completed_lessons
 FROM course_lesson_progress p JOIN course_lessons l ON l.id = p.lesson_id
 WHERE p.user_id = ar.member_id AND l.course_id = ac.course_id AND l.deleted_at IS NULL
) progress;

CREATE TABLE custom_form_sessions (
 id uuid PRIMARY KEY, form_id integer NOT NULL REFERENCES custom_forms(id) ON DELETE CASCADE,
 token_hash varchar(64) NOT NULL UNIQUE, user_id integer REFERENCES public_users(id) ON DELETE SET NULL,
 schema_hash varchar(64) NOT NULL, expires_at timestamptz NOT NULL, completed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE custom_form_responses (
 id uuid PRIMARY KEY, form_id integer NOT NULL REFERENCES custom_forms(id) ON DELETE RESTRICT,
 session_id uuid NOT NULL UNIQUE REFERENCES custom_form_sessions(id) ON DELETE RESTRICT,
 user_id integer REFERENCES public_users(id) ON DELETE SET NULL, form_snapshot jsonb NOT NULL,
 answers jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE custom_form_attachments (
 id uuid PRIMARY KEY, session_id uuid NOT NULL REFERENCES custom_form_sessions(id) ON DELETE RESTRICT,
 field_key varchar(255) NOT NULL, storage_key varchar(512) NOT NULL UNIQUE,
 original_name varchar(255) NOT NULL, download_name varchar(255) NOT NULL,
 mime_type varchar(100) NOT NULL, size_bytes integer NOT NULL, source_size_bytes integer NOT NULL,
 width integer, height integer, claimed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE public.calendar_events (
    id serial PRIMARY KEY,
    title varchar(255) NOT NULL CHECK (length(trim(title)) > 0),
    description text,
    location varchar(500),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    all_day boolean NOT NULL DEFAULT false,
    activity_id integer REFERENCES public.activities(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);
CREATE INDEX calendar_events_starts_at_index ON public.calendar_events(starts_at);
CREATE INDEX calendar_events_ends_at_index ON public.calendar_events(ends_at);
CREATE INDEX calendar_events_activity_id_index ON public.calendar_events(activity_id);

CREATE TABLE public.announcements (
 id serial PRIMARY KEY, title varchar(160) NOT NULL, body text NOT NULL,
 link_label varchar(100), link_url text, audience jsonb NOT NULL,
 state varchar(20) NOT NULL DEFAULT 'draft', version integer NOT NULL DEFAULT 1,
 author_id integer REFERENCES public.admin_users(id) ON DELETE SET NULL,
 publisher_id integer REFERENCES public.admin_users(id) ON DELETE SET NULL,
 published_at timestamptz, withdrawn_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 recipient_count integer NOT NULL DEFAULT 0,
 CHECK (state IN ('draft','published','withdrawn')),
 CHECK (length(trim(title)) > 0 AND length(trim(body)) BETWEEN 1 AND 10000),
 CHECK ((link_label IS NULL) = (link_url IS NULL))
);
CREATE TABLE public.announcement_recipients (
 id serial PRIMARY KEY, announcement_id integer NOT NULL REFERENCES public.announcements(id) ON DELETE CASCADE,
 admin_user_id integer REFERENCES public.admin_users(id) ON DELETE CASCADE,
 public_user_id integer REFERENCES public.public_users(id) ON DELETE CASCADE,
 read_at timestamptz,
 CHECK ((admin_user_id IS NOT NULL) <> (public_user_id IS NOT NULL)),
 UNIQUE(announcement_id, admin_user_id), UNIQUE(announcement_id, public_user_id)
);
CREATE INDEX ON public.announcement_recipients(admin_user_id, read_at, id);
CREATE INDEX ON public.announcement_recipients(public_user_id, read_at, id);

-- Case-insensitive public account lookup; built concurrently by Ace.
CREATE INDEX idx_public_users_lower_email ON public.public_users USING btree (lower((email)::text));

CREATE TABLE public.talent_assessment_drafts (
 admin_user_id integer PRIMARY KEY REFERENCES public.admin_users(id) ON DELETE CASCADE,
 draft_id text NOT NULL UNIQUE,
 definition_version text NOT NULL,
 answers jsonb NOT NULL CHECK (jsonb_typeof(answers) = 'array' AND jsonb_array_length(answers) = 170),
 current_question integer NOT NULL DEFAULT 1 CHECK (current_question BETWEEN 1 AND 170),
 revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.talent_assessment_results (
 admin_user_id integer PRIMARY KEY REFERENCES public.admin_users(id) ON DELETE CASCADE,
 submission_id text NOT NULL UNIQUE,
 definition_version text NOT NULL,
 answers jsonb NOT NULL CHECK (jsonb_typeof(answers) = 'array' AND jsonb_array_length(answers) = 170),
 result jsonb NOT NULL,
 submitted_at timestamptz NOT NULL DEFAULT now()
);

-- Pembinaan Beasiswa Perintis (Ace migration 1793168600200_create_perintis_tables).
CREATE TABLE public.perintis_participants (
  user_id integer PRIMARY KEY REFERENCES public.public_users(id) ON DELETE RESTRICT,
  is_mentee boolean NOT NULL DEFAULT false,
  is_mentor boolean NOT NULL DEFAULT false,
  is_coordinator boolean NOT NULL DEFAULT false,

  nim varchar(30),
  university_id integer REFERENCES public.universities(id) ON DELETE SET NULL,
  kip_k boolean,
  category varchar(20),
  kaderisasi_override varchar(20),
  kaderisasi_override_note text,
  mentor_user_id integer REFERENCES public.perintis_participants(user_id) ON DELETE SET NULL,

  coordinator_user_id integer REFERENCES public.perintis_participants(user_id) ON DELETE SET NULL,

  status varchar(20) NOT NULL DEFAULT 'active',
  inactive_reason text,
  updated_by integer REFERENCES public.admin_users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  UNIQUE (university_id, nim),
  CHECK (is_mentee OR is_mentor OR is_coordinator),
  CHECK (is_mentee OR (nim IS NULL AND mentor_user_id IS NULL)),
  CHECK (is_mentor OR coordinator_user_id IS NULL),
  CHECK (category IS NULL OR category IN ('dasar','biaya_hidup')),
  CHECK (kaderisasi_override IS NULL OR kaderisasi_override IN ('lmd_lama','lmd_242','setara','belum')),
  CHECK (mentor_user_id IS NULL OR mentor_user_id <> user_id),
  CHECK (status IN ('active','inactive')),
  CHECK ((status = 'inactive') = (inactive_reason IS NOT NULL))
);
CREATE INDEX perintis_participants_mentor_idx ON public.perintis_participants (mentor_user_id) WHERE is_mentee;
CREATE INDEX perintis_participants_coordinator_idx ON public.perintis_participants (coordinator_user_id) WHERE is_mentor;

CREATE TABLE public.perintis_journals (
  id serial PRIMARY KEY,
  mentee_user_id integer NOT NULL REFERENCES public.perintis_participants(user_id) ON DELETE RESTRICT,
  month date NOT NULL CHECK (extract(day FROM month) = 1 AND month >= DATE '2026-11-01'),
  mentor_user_id integer REFERENCES public.perintis_participants(user_id) ON DELETE SET NULL,
  category varchar(20) NOT NULL,
  form_version integer NOT NULL,

  status varchar(25) NOT NULL DEFAULT 'draft',
  answers jsonb NOT NULL DEFAULT '{}'::jsonb,
  files jsonb NOT NULL DEFAULT '[]'::jsonb,
  pulse_energy smallint, pulse_academic smallint, pulse_finance smallint, pulse_relation smallint,
  wants_contact boolean NOT NULL DEFAULT false,
  honesty_confirmed_at timestamptz,
  submitted_at timestamptz,
  is_late boolean NOT NULL DEFAULT false,
  revision integer NOT NULL DEFAULT 1,
  reopened_by integer REFERENCES public.admin_users(id) ON DELETE SET NULL,
  reopen_reason text,

  response_appreciation text,
  response_text text,
  revision_request text,
  flag_reason text,
  responded_by integer REFERENCES public.public_users(id) ON DELETE SET NULL,
  responded_at timestamptz,
  response_read_at timestamptz,
  ai_assisted boolean NOT NULL DEFAULT false,

  flag_trigger varchar(20),
  flag_status varchar(20),
  flag_due_at timestamptz,
  flag_handled_by integer REFERENCES public.public_users(id) ON DELETE SET NULL,
  flag_handled_at timestamptz,
  flag_private_note text,

  followup_by integer REFERENCES public.admin_users(id) ON DELETE SET NULL,
  followup_at timestamptz,
  followup_note text,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (mentee_user_id, month),
  CHECK (category IN ('dasar','biaya_hidup')),
  CHECK (status IN ('draft','submitted','accepted','revision_requested','revised','flagged')),
  CHECK (jsonb_typeof(answers) = 'object' AND jsonb_typeof(files) = 'array'),
  CHECK (status = 'draft' OR (submitted_at IS NOT NULL AND honesty_confirmed_at IS NOT NULL)),
  CHECK (pulse_energy BETWEEN 1 AND 5 AND pulse_academic BETWEEN 1 AND 5 AND pulse_finance BETWEEN 1 AND 5 AND pulse_relation BETWEEN 1 AND 5),
  CHECK ((reopened_by IS NULL) = (reopen_reason IS NULL)),
  CHECK (status NOT IN ('accepted','revision_requested','flagged')
         OR (response_appreciation IS NOT NULL AND response_text IS NOT NULL AND responded_at IS NOT NULL)),
  CHECK (status <> 'revision_requested' OR revision_request IS NOT NULL),
  CHECK (status <> 'flagged' OR flag_reason IS NOT NULL),
  CHECK ((flag_trigger IS NULL) = (flag_status IS NULL) AND (flag_trigger IS NULL) = (flag_due_at IS NULL)),
  CHECK (flag_trigger IS NULL OR flag_trigger IN ('low_pulse','contact_request','both')),
  CHECK (flag_status IS NULL OR flag_status IN ('open','handled','escalated','resolved')),
  CHECK ((flag_status = 'handled') = (flag_handled_at IS NOT NULL)),
  CHECK (flag_status IS DISTINCT FROM 'resolved' OR followup_at IS NOT NULL),
  CHECK ((followup_at IS NULL) = (followup_note IS NULL))
);
CREATE INDEX perintis_journals_mentor_month_idx ON public.perintis_journals (mentor_user_id, month, status);
CREATE INDEX perintis_journals_month_status_idx ON public.perintis_journals (month, status);
CREATE INDEX perintis_journals_open_flag_idx ON public.perintis_journals (flag_due_at) WHERE flag_status IN ('open','escalated');
CREATE INDEX perintis_journals_followup_idx ON public.perintis_journals (submitted_at)
  WHERE (status = 'flagged' OR flag_status = 'escalated') AND followup_at IS NULL;

CREATE TABLE public.perintis_session_reports (
  id serial PRIMARY KEY,
  mentor_user_id integer NOT NULL REFERENCES public.perintis_participants(user_id) ON DELETE RESTRICT,
  month date NOT NULL CHECK (extract(day FROM month) = 1 AND month >= DATE '2026-11-01'),

  session_date date,
  mode varchar(10) NOT NULL DEFAULT 'luring',
  location_or_link varchar(500),
  curriculum_code varchar(20),
  deviation_reason text,
  summary text,
  photos jsonb NOT NULL DEFAULT '[]'::jsonb,

  case_note text,
  case_status varchar(10),
  followup_by integer REFERENCES public.admin_users(id) ON DELETE SET NULL,
  followup_at timestamptz,
  followup_note text,

  status varchar(20) NOT NULL DEFAULT 'draft',
  submitted_at timestamptz,
  is_late boolean NOT NULL DEFAULT false,

  review_appreciation text,
  review_note text,
  reviewer_kind varchar(10),
  reviewed_by integer,
  reviewed_at timestamptz,
  review_read_at timestamptz,
  verified_at timestamptz,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (mentor_user_id, month),
  CHECK (mode IN ('luring','daring')),
  CHECK (jsonb_typeof(photos) = 'array'),
  CHECK (status IN ('draft','submitted','returned','verified')),
  CHECK (status = 'draft' OR (submitted_at IS NOT NULL AND session_date IS NOT NULL AND jsonb_array_length(photos) > 0)),
  CHECK ((status = 'verified') = (verified_at IS NOT NULL)),
  CHECK (status NOT IN ('returned','verified') OR (review_appreciation IS NOT NULL AND review_note IS NOT NULL AND reviewed_at IS NOT NULL)),
  CHECK (reviewer_kind IS NULL OR reviewer_kind IN ('public','admin')),
  CHECK ((case_note IS NULL) = (case_status IS NULL)),
  CHECK (case_status IS NULL OR case_status IN ('open','resolved')),
  CHECK ((case_status = 'resolved') = (followup_at IS NOT NULL AND followup_note IS NOT NULL))
);
CREATE INDEX perintis_session_reports_month_idx ON public.perintis_session_reports (month, status);
CREATE INDEX perintis_session_reports_open_case_idx ON public.perintis_session_reports (created_at) WHERE case_status = 'open';

-- P3RI committee registration (Ace migrations 1793195171058_create_p3ri_registrations_table,
-- 1793205707665_narrow_p3ri_registration_statuses).
CREATE TABLE public.p3ri_registrations (
  id serial PRIMARY KEY,
  user_id integer NOT NULL UNIQUE REFERENCES public.public_users(id) ON DELETE RESTRICT,
  status varchar(20) NOT NULL DEFAULT 'terdaftar',
  choices text[] NOT NULL,
  accepted_division varchar(60),
  answers jsonb NOT NULL DEFAULT '{}'::jsonb,
  files jsonb NOT NULL DEFAULT '[]'::jsonb,
  admin_note text,
  status_updated_by integer REFERENCES public.admin_users(id) ON DELETE SET NULL,
  status_updated_at timestamptz,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT p3ri_registrations_status_check CHECK (status IN ('terdaftar','diterima')),
  CHECK (cardinality(choices) BETWEEN 1 AND 3),
  CHECK ((status = 'diterima') = (accepted_division IS NOT NULL)),
  CHECK (accepted_division IS NULL OR accepted_division = ANY (choices)),
  CHECK (jsonb_typeof(answers) = 'object' AND jsonb_typeof(files) = 'array')
);
CREATE INDEX p3ri_registrations_status_idx ON public.p3ri_registrations (status);
CREATE INDEX p3ri_registrations_choices_idx ON public.p3ri_registrations USING gin (choices);
