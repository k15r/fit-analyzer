package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"gopkg.in/yaml.v3"
)

// ---- sentinel helpers ----

func validU8(v uint8) bool   { return v != 0xFF }
func validU16(v uint16) bool { return v != 0xFFFF }
func validU32(v uint32) bool { return v != 0xFFFFFFFF }
func validI8(v int8) bool    { return v != -128 && v != 127 }
func validI16(v int16) bool  { return v != math.MinInt16 }

func optU8(v uint8) *int {
	if !validU8(v) {
		return nil
	}
	x := int(v)
	return &x
}
func optU16(v uint16) *int {
	if !validU16(v) {
		return nil
	}
	x := int(v)
	return &x
}
func optI8(v int8) *int {
	if !validI8(v) {
		return nil
	}
	x := int(v)
	return &x
}
func optFloat(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// scale helpers — return nil on invalid sentinel
func optSpeed32(raw uint32) *float64 { // m/s, scale 1000
	if !validU32(raw) {
		return nil
	}
	v := float64(raw) / 1000.0
	return &v
}
func optSpeed16(raw uint16) *float64 { // m/s, scale 1000
	if !validU16(raw) {
		return nil
	}
	v := float64(raw) / 1000.0
	return &v
}
func optDist32(raw uint32) *float64 { // metres, scale 100
	if !validU32(raw) {
		return nil
	}
	v := float64(raw) / 100.0
	return &v
}
func optTime32(raw uint32) *float64 { // seconds, scale 1000
	if !validU32(raw) {
		return nil
	}
	v := float64(raw) / 1000.0
	return &v
}
func optAlt32(raw uint32) *float64 { // metres, scale 5, offset -500
	if !validU32(raw) {
		return nil
	}
	v := float64(raw)*0.2 - 500
	return &v
}
func optAlt16(raw uint16) *float64 { // metres, scale 5, offset -500
	if !validU16(raw) {
		return nil
	}
	v := float64(raw)*0.2 - 500
	return &v
}
func optResp(raw uint16) *float64 { // breaths/min, scale 100
	if !validU16(raw) {
		return nil
	}
	v := float64(raw) / 100.0
	return &v
}
func optTrainingEffect(raw uint8) *float64 { // scale 10
	if !validU8(raw) {
		return nil
	}
	v := float64(raw) / 10.0
	return &v
}
func optStepLength(raw uint16) *float64 { // mm, scale 10
	if !validU16(raw) {
		return nil
	}
	v := float64(raw) / 10.0
	return &v
}
func optVertOsc(raw uint16) *float64 { // mm, scale 10
	if !validU16(raw) {
		return nil
	}
	v := float64(raw) / 10.0
	return &v
}
func optStanceTime(raw uint16) *float64 { // ms, scale 10
	if !validU16(raw) {
		return nil
	}
	v := float64(raw) / 10.0
	return &v
}
func optVertRatio(raw uint16) *float64 { // %, scale 100
	if !validU16(raw) {
		return nil
	}
	v := float64(raw) / 100.0
	return &v
}
func optWeight(raw uint16) *float64 { // kg, scale 10
	if !validU16(raw) {
		return nil
	}
	v := float64(raw) / 10.0
	return &v
}
func optHeight(raw uint8) *float64 { // m, scale 100 (stored as cm essentially)
	if !validU8(raw) {
		return nil
	}
	v := float64(raw) / 100.0
	return &v
}

// ---- pace / duration formatters ----

func formatDuration(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func pace(speedMperS float64) string {
	if speedMperS <= 0 {
		return "N/A"
	}
	secsPerKm := 1000.0 / speedMperS
	m := int(secsPerKm) / 60
	s := int(secsPerKm) % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func paceFromDistTime(distM, secs float64) string {
	if distM > 0 && secs > 0 {
		return pace(distM / secs)
	}
	return "N/A"
}

// bestPace picks the first valid speed source; falls back to dist/time.
func bestPace(speed32 uint32, speed16 uint16, distM, secs float64) string {
	if s := optSpeed32(speed32); s != nil && *s > 0 {
		return pace(*s)
	}
	if s := optSpeed16(speed16); s != nil && *s > 0 {
		return pace(*s)
	}
	return paceFromDistTime(distM, secs)
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// ---- output types ----

type Output struct {
	UserProfile     *UserProfile     `json:"user_profile,omitempty" yaml:"user_profile,omitempty"`
	Metadata        Metadata         `json:"metadata" yaml:"metadata"`
	RunningDynamics *RunningDynamics `json:"running_dynamics,omitempty" yaml:"running_dynamics,omitempty"`
	Workout         *WorkoutInfo     `json:"workout,omitempty" yaml:"workout,omitempty"`
	Laps            []LapStats       `json:"laps" yaml:"laps"`
	Splits          []SplitStats     `json:"splits,omitempty" yaml:"splits,omitempty"`
	SplitSummaries  []SplitSummary   `json:"split_summaries,omitempty" yaml:"split_summaries,omitempty"`
	HeightProfile   *HeightProfile   `json:"height_profile,omitempty" yaml:"height_profile,omitempty"`
	GpsTrack        *GpsTrack        `json:"gps_track,omitempty" yaml:"gps_track,omitempty"`
}

type GpsPoint struct {
	ElapsedSec   int      `json:"elapsed_sec" yaml:"elapsed_sec"`
	DistanceKm   float64  `json:"distance_km" yaml:"distance_km"`
	Lat          float64  `json:"lat" yaml:"lat"`
	Lon          float64  `json:"lon" yaml:"lon"`
	AltitudeFitM *float64 `json:"altitude_fit_m,omitempty" yaml:"altitude_fit_m,omitempty"`
	AltitudeApiM *float64 `json:"altitude_api_m,omitempty" yaml:"altitude_api_m,omitempty"`
}

type GpsTrack struct {
	IntervalSec  int        `json:"interval_sec,omitempty" yaml:"interval_sec,omitempty"`
	IntervalM    int        `json:"interval_m,omitempty" yaml:"interval_m,omitempty"`
	Points       []GpsPoint `json:"points" yaml:"points"`
}

type HeightProfilePoint struct {
	DistanceKm float64 `json:"distance_km" yaml:"distance_km"`
	AltitudeM  float64 `json:"altitude_m" yaml:"altitude_m"`
}

type KmElevation struct {
	KmMark int `json:"km" yaml:"km"`
	GainM  int `json:"gain_m" yaml:"gain_m"`
	LossM  int `json:"loss_m" yaml:"loss_m"`
}

type HeightProfile struct {
	MinAltitudeM float64              `json:"min_altitude_m" yaml:"min_altitude_m"`
	MaxAltitudeM float64              `json:"max_altitude_m" yaml:"max_altitude_m"`
	Points       []HeightProfilePoint `json:"points" yaml:"points"`
	Sparkline    string               `json:"sparkline" yaml:"sparkline"`
	KmElevation  []KmElevation        `json:"km_elevation,omitempty" yaml:"km_elevation,omitempty"`
	SvgPath      string               `json:"svg_path,omitempty" yaml:"svg_path,omitempty"`
}

type UserProfile struct {
	Name              string   `json:"name,omitempty" yaml:"name,omitempty"`
	Gender            string   `json:"gender,omitempty" yaml:"gender,omitempty"`
	WeightKg          *float64 `json:"weight_kg,omitempty" yaml:"weight_kg,omitempty"`
	HeightM           *float64 `json:"height_m,omitempty" yaml:"height_m,omitempty"`
	RestingHeartRate  *int     `json:"resting_heart_rate_bpm,omitempty" yaml:"resting_heart_rate_bpm,omitempty"`
	MaxHeartRate      *int     `json:"max_heart_rate_bpm,omitempty" yaml:"max_heart_rate_bpm,omitempty"`
}

type Metadata struct {
	Sport         string    `json:"sport" yaml:"sport"`
	SubSport      string    `json:"sub_sport" yaml:"sub_sport"`
	StartTime     time.Time `json:"start_time" yaml:"start_time"`
	TotalDistance float64   `json:"total_distance_km" yaml:"total_distance_km"`
	TotalDuration string    `json:"total_duration" yaml:"total_duration"`
	MovingTime    *string   `json:"moving_time,omitempty" yaml:"moving_time,omitempty"`
	TotalCalories int       `json:"total_calories" yaml:"total_calories"`
	AvgHeartRate  *int      `json:"avg_heart_rate_bpm,omitempty" yaml:"avg_heart_rate_bpm,omitempty"`
	MaxHeartRate  *int      `json:"max_heart_rate_bpm,omitempty" yaml:"max_heart_rate_bpm,omitempty"`
	TotalAscent   *int      `json:"total_ascent_m,omitempty" yaml:"total_ascent_m,omitempty"`
	TotalDescent  *int      `json:"total_descent_m,omitempty" yaml:"total_descent_m,omitempty"`
	AvgPace       string    `json:"avg_pace_min_per_km" yaml:"avg_pace_min_per_km"`
	MaxSpeed      *float64  `json:"max_speed_m_s,omitempty" yaml:"max_speed_m_s,omitempty"`
	AvgCadence    *int      `json:"avg_cadence_spm,omitempty" yaml:"avg_cadence_spm,omitempty"`
	MaxCadence    *int      `json:"max_cadence_spm,omitempty" yaml:"max_cadence_spm,omitempty"`
	AvgTemperature *int     `json:"avg_temperature_c,omitempty" yaml:"avg_temperature_c,omitempty"`
	MaxTemperature *int     `json:"max_temperature_c,omitempty" yaml:"max_temperature_c,omitempty"`
	AvgAltitude   *float64  `json:"avg_altitude_m,omitempty" yaml:"avg_altitude_m,omitempty"`
	MaxAltitude   *float64  `json:"max_altitude_m,omitempty" yaml:"max_altitude_m,omitempty"`
	MinAltitude   *float64  `json:"min_altitude_m,omitempty" yaml:"min_altitude_m,omitempty"`
	TrainingEffect *float64 `json:"training_effect,omitempty" yaml:"training_effect,omitempty"`
	AnaerobicEffect *float64 `json:"anaerobic_training_effect,omitempty" yaml:"anaerobic_training_effect,omitempty"`
	WorkoutFeel   *int      `json:"workout_feel,omitempty" yaml:"workout_feel,omitempty"`
	WorkoutRpe    *int      `json:"workout_rpe,omitempty" yaml:"workout_rpe,omitempty"`
	AvgSpo2       *int      `json:"avg_spo2_pct,omitempty" yaml:"avg_spo2_pct,omitempty"`
	AvgStress     *int      `json:"avg_stress,omitempty" yaml:"avg_stress,omitempty"`
	HrvSdrr       *int      `json:"hrv_sdrr_ms,omitempty" yaml:"hrv_sdrr_ms,omitempty"`
	HrvRmssd      *int      `json:"hrv_rmssd_ms,omitempty" yaml:"hrv_rmssd_ms,omitempty"`
	AvgRespiration *float64 `json:"avg_respiration_breaths_min,omitempty" yaml:"avg_respiration_breaths_min,omitempty"`
	MaxRespiration *float64 `json:"max_respiration_breaths_min,omitempty" yaml:"max_respiration_breaths_min,omitempty"`
	NormalizedPower *int    `json:"normalized_power_w,omitempty" yaml:"normalized_power_w,omitempty"`
}

type RunningDynamics struct {
	AvgStepLengthMm       *float64 `json:"avg_step_length_mm,omitempty" yaml:"avg_step_length_mm,omitempty"`
	AvgVerticalOscMm      *float64 `json:"avg_vertical_oscillation_mm,omitempty" yaml:"avg_vertical_oscillation_mm,omitempty"`
	AvgStanceTimeMs       *float64 `json:"avg_stance_time_ms,omitempty" yaml:"avg_stance_time_ms,omitempty"`
	AvgVerticalRatioPct   *float64 `json:"avg_vertical_ratio_pct,omitempty" yaml:"avg_vertical_ratio_pct,omitempty"`
}

type WorkoutInfo struct {
	Name  string         `json:"name" yaml:"name"`
	Sport string         `json:"sport" yaml:"sport"`
	Steps []WorkoutStep  `json:"steps,omitempty" yaml:"steps,omitempty"`
}

type WorkoutStep struct {
	Name         string `json:"name,omitempty" yaml:"name,omitempty"`
	DurationType string `json:"duration_type" yaml:"duration_type"`
	DurationValue uint32 `json:"duration_value" yaml:"duration_value"`
	TargetType   string `json:"target_type" yaml:"target_type"`
	TargetValue  uint32 `json:"target_value" yaml:"target_value"`
	Notes        string `json:"notes,omitempty" yaml:"notes,omitempty"`
}

type LapStats struct {
	Number         int      `json:"number" yaml:"number"`
	Trigger        string   `json:"trigger" yaml:"trigger"`
	Distance       float64  `json:"distance_km" yaml:"distance_km"`
	Duration       string   `json:"duration" yaml:"duration"`
	Pace           string   `json:"pace_min_per_km" yaml:"pace_min_per_km"`
	Calories       *int     `json:"calories,omitempty" yaml:"calories,omitempty"`
	AvgHeartRate   *int     `json:"avg_heart_rate_bpm,omitempty" yaml:"avg_heart_rate_bpm,omitempty"`
	MaxHeartRate   *int     `json:"max_heart_rate_bpm,omitempty" yaml:"max_heart_rate_bpm,omitempty"`
	ElevationGain  *int     `json:"elevation_gain_m,omitempty" yaml:"elevation_gain_m,omitempty"`
	ElevationLoss  *int     `json:"elevation_loss_m,omitempty" yaml:"elevation_loss_m,omitempty"`
	MinAltitude    *float64 `json:"min_altitude_m,omitempty" yaml:"min_altitude_m,omitempty"`
	MaxAltitude    *float64 `json:"max_altitude_m,omitempty" yaml:"max_altitude_m,omitempty"`
	AvgCadence     *int     `json:"avg_cadence_spm,omitempty" yaml:"avg_cadence_spm,omitempty"`
	MaxCadence     *int     `json:"max_cadence_spm,omitempty" yaml:"max_cadence_spm,omitempty"`
	AvgTemperature *int     `json:"avg_temperature_c,omitempty" yaml:"avg_temperature_c,omitempty"`
	MaxTemperature *int     `json:"max_temperature_c,omitempty" yaml:"max_temperature_c,omitempty"`
	AvgPower       *int     `json:"avg_power_w,omitempty" yaml:"avg_power_w,omitempty"`
	MaxPower       *int     `json:"max_power_w,omitempty" yaml:"max_power_w,omitempty"`
	NormalizedPower *int    `json:"normalized_power_w,omitempty" yaml:"normalized_power_w,omitempty"`
	AvgRespiration *float64 `json:"avg_respiration_breaths_min,omitempty" yaml:"avg_respiration_breaths_min,omitempty"`
	RunningDynamics *RunningDynamics `json:"running_dynamics,omitempty" yaml:"running_dynamics,omitempty"`
}

type SplitStats struct {
	Number        int      `json:"number" yaml:"number"`
	Type          string   `json:"type" yaml:"type"`
	Distance      float64  `json:"distance_km" yaml:"distance_km"`
	Duration      string   `json:"duration" yaml:"duration"`
	Pace          string   `json:"pace_min_per_km" yaml:"pace_min_per_km"`
	ElevationGain *int     `json:"elevation_gain_m,omitempty" yaml:"elevation_gain_m,omitempty"`
	ElevationLoss *int     `json:"elevation_loss_m,omitempty" yaml:"elevation_loss_m,omitempty"`
	Calories      *int     `json:"calories,omitempty" yaml:"calories,omitempty"`
}

type SplitSummary struct {
	Type          string   `json:"type" yaml:"type"`
	NumSplits     int      `json:"num_splits" yaml:"num_splits"`
	Distance      float64  `json:"distance_km" yaml:"distance_km"`
	Duration      string   `json:"duration" yaml:"duration"`
	Pace          string   `json:"pace_min_per_km" yaml:"pace_min_per_km"`
	AvgHeartRate  *int     `json:"avg_heart_rate_bpm,omitempty" yaml:"avg_heart_rate_bpm,omitempty"`
	MaxHeartRate  *int     `json:"max_heart_rate_bpm,omitempty" yaml:"max_heart_rate_bpm,omitempty"`
	ElevationGain *int     `json:"elevation_gain_m,omitempty" yaml:"elevation_gain_m,omitempty"`
	ElevationLoss *int     `json:"elevation_loss_m,omitempty" yaml:"elevation_loss_m,omitempty"`
	Calories      *int     `json:"calories,omitempty" yaml:"calories,omitempty"`
}

// ---- builders ----

func buildUserProfile(u *mesgdef.UserProfile) *UserProfile {
	if u == nil {
		return nil
	}
	p := &UserProfile{}
	if u.FriendlyName != "" {
		p.Name = u.FriendlyName
	}
	gender := u.Gender.String()
	if gender != "" && gender != "GenderInvalid(255)" {
		p.Gender = gender
	}
	p.WeightKg = optWeight(u.Weight)
	// Height is uint8 in cm (e.g. 182 = 1.82m)
	if validU8(u.Height) {
		v := float64(u.Height) / 100.0
		p.HeightM = &v
	}
	p.RestingHeartRate = optU8(u.RestingHeartRate)
	p.MaxHeartRate = optU8(u.DefaultMaxHeartRate)
	return p
}

func buildMetadata(sess *mesgdef.Session) Metadata {
	elapsed := *optTime32(sess.TotalElapsedTime)
	dist := float64(0)
	if d := optDist32(sess.TotalDistance); d != nil {
		dist = *d / 1000.0
	}

	avgPace := bestPace(sess.EnhancedAvgSpeed, sess.AvgSpeed, dist*1000, elapsed)

	m := Metadata{
		Sport:         sess.Sport.String(),
		SubSport:      sess.SubSport.String(),
		StartTime:     sess.StartTime,
		TotalDistance: round3(dist),
		TotalDuration: formatDuration(elapsed),
		TotalCalories: int(sess.TotalCalories),
		AvgPace:       avgPace,
	}

	m.AvgHeartRate = optU8(sess.AvgHeartRate)
	m.MaxHeartRate = optU8(sess.MaxHeartRate)

	if validU16(sess.TotalAscent) {
		v := int(sess.TotalAscent)
		m.TotalAscent = &v
	}
	if validU16(sess.TotalDescent) {
		v := int(sess.TotalDescent)
		m.TotalDescent = &v
	}

	if s := optSpeed32(sess.EnhancedMaxSpeed); s != nil {
		m.MaxSpeed = s
	} else {
		m.MaxSpeed = optSpeed16(sess.MaxSpeed)
	}

	if t := optTime32(sess.TotalMovingTime); t != nil {
		s := formatDuration(*t)
		m.MovingTime = &s
	}

	m.AvgCadence = optU8(sess.AvgCadence)
	m.MaxCadence = optU8(sess.MaxCadence)
	m.AvgTemperature = optI8(sess.AvgTemperature)
	m.MaxTemperature = optI8(sess.MaxTemperature)

	m.AvgAltitude = optAlt32(sess.EnhancedAvgAltitude)
	m.MaxAltitude = optAlt32(sess.EnhancedMaxAltitude)
	m.MinAltitude = optAlt32(sess.EnhancedMinAltitude)

	m.TrainingEffect = optTrainingEffect(sess.TotalTrainingEffect)
	m.AnaerobicEffect = optTrainingEffect(sess.TotalAnaerobicTrainingEffect)

	m.WorkoutFeel = optU8(sess.WorkoutFeel)
	m.WorkoutRpe = optU8(sess.WorkoutRpe)
	m.AvgSpo2 = optU8(sess.AvgSpo2)
	m.AvgStress = optU8(sess.AvgStress)
	m.HrvSdrr = optU8(sess.SdrrHrv)
	m.HrvRmssd = optU8(sess.RmssdHrv)

	m.AvgRespiration = optResp(sess.EnhancedAvgRespirationRate)
	m.MaxRespiration = optResp(sess.EnhancedMaxRespirationRate)

	if validU16(sess.NormalizedPower) {
		v := int(sess.NormalizedPower)
		m.NormalizedPower = &v
	}

	return m
}

func buildSessionDynamics(sess *mesgdef.Session) *RunningDynamics {
	rd := &RunningDynamics{
		AvgStepLengthMm:     optStepLength(sess.AvgStepLength),
		AvgVerticalOscMm:    optVertOsc(sess.AvgVerticalOscillation),
		AvgStanceTimeMs:     optStanceTime(sess.AvgStanceTime),
		AvgVerticalRatioPct: optVertRatio(sess.AvgVerticalRatio),
	}
	if rd.AvgStepLengthMm == nil && rd.AvgVerticalOscMm == nil &&
		rd.AvgStanceTimeMs == nil && rd.AvgVerticalRatioPct == nil {
		return nil
	}
	return rd
}

func buildLapDynamics(lap *mesgdef.Lap) *RunningDynamics {
	rd := &RunningDynamics{
		AvgStepLengthMm:     optStepLength(lap.AvgStepLength),
		AvgVerticalOscMm:    optVertOsc(lap.AvgVerticalOscillation),
		AvgStanceTimeMs:     optStanceTime(lap.AvgStanceTime),
		AvgVerticalRatioPct: optVertRatio(lap.AvgVerticalRatio),
	}
	if rd.AvgStepLengthMm == nil && rd.AvgVerticalOscMm == nil &&
		rd.AvgStanceTimeMs == nil && rd.AvgVerticalRatioPct == nil {
		return nil
	}
	return rd
}

func buildLapStats(n int, lap *mesgdef.Lap) LapStats {
	dist := float64(0)
	if d := optDist32(lap.TotalDistance); d != nil {
		dist = *d / 1000.0
	}
	elapsed := float64(0)
	if t := optTime32(lap.TotalTimerTime); t != nil {
		elapsed = *t
	}

	ls := LapStats{
		Number:   n,
		Trigger:  lap.LapTrigger.String(),
		Distance: round3(dist),
		Duration: formatDuration(elapsed),
		Pace:     bestPace(lap.EnhancedAvgSpeed, lap.AvgSpeed, dist*1000, elapsed),
	}

	ls.Calories = optU16(lap.TotalCalories)
	ls.AvgHeartRate = optU8(lap.AvgHeartRate)
	ls.MaxHeartRate = optU8(lap.MaxHeartRate)

	if validU16(lap.TotalAscent) {
		v := int(lap.TotalAscent)
		ls.ElevationGain = &v
	}
	if validU16(lap.TotalDescent) {
		v := int(lap.TotalDescent)
		ls.ElevationLoss = &v
	}
	ls.MinAltitude = optAlt32(lap.EnhancedMinAltitude)
	ls.MaxAltitude = optAlt32(lap.EnhancedMaxAltitude)

	ls.AvgCadence = optU8(lap.AvgCadence)
	ls.MaxCadence = optU8(lap.MaxCadence)
	ls.AvgTemperature = optI8(lap.AvgTemperature)
	ls.MaxTemperature = optI8(lap.MaxTemperature)

	if validU16(lap.AvgPower) {
		v := int(lap.AvgPower)
		ls.AvgPower = &v
	}
	if validU16(lap.MaxPower) {
		v := int(lap.MaxPower)
		ls.MaxPower = &v
	}
	if validU16(lap.NormalizedPower) {
		v := int(lap.NormalizedPower)
		ls.NormalizedPower = &v
	}

	ls.AvgRespiration = optResp(lap.EnhancedAvgRespirationRate)
	ls.RunningDynamics = buildLapDynamics(lap)

	return ls
}

func buildSplitStats(n int, s *mesgdef.Split) SplitStats {
	dist := float64(0)
	if d := optDist32(s.TotalDistance); d != nil {
		dist = *d / 1000.0
	}
	elapsed := float64(0)
	if t := optTime32(s.TotalElapsedTime); t != nil {
		elapsed = *t
	}
	p := "N/A"
	if spd := optSpeed32(s.AvgSpeed); spd != nil && *spd > 0 {
		p = pace(*spd)
	} else {
		p = paceFromDistTime(dist*1000, elapsed)
	}

	ss := SplitStats{
		Number:   n,
		Type:     s.SplitType.String(),
		Distance: round3(dist),
		Duration: formatDuration(elapsed),
		Pace:     p,
	}
	if validU16(s.TotalAscent) {
		v := int(s.TotalAscent)
		ss.ElevationGain = &v
	}
	if validU16(s.TotalDescent) {
		v := int(s.TotalDescent)
		ss.ElevationLoss = &v
	}
	if validU32(s.TotalCalories) {
		v := int(s.TotalCalories)
		ss.Calories = &v
	}
	return ss
}

func buildSplitSummary(s *mesgdef.SplitSummary) SplitSummary {
	dist := float64(0)
	if d := optDist32(s.TotalDistance); d != nil {
		dist = *d / 1000.0
	}
	elapsed := float64(0)
	if t := optTime32(s.TotalTimerTime); t != nil {
		elapsed = *t
	}
	p := "N/A"
	if spd := optSpeed32(s.AvgSpeed); spd != nil && *spd > 0 {
		p = pace(*spd)
	} else {
		p = paceFromDistTime(dist*1000, elapsed)
	}

	ss := SplitSummary{
		Type:      s.SplitType.String(),
		NumSplits: int(s.NumSplits),
		Distance:  round3(dist),
		Duration:  formatDuration(elapsed),
		Pace:      p,
	}
	ss.AvgHeartRate = optU8(s.AvgHeartRate)
	ss.MaxHeartRate = optU8(s.MaxHeartRate)
	if validU16(s.TotalAscent) {
		v := int(s.TotalAscent)
		ss.ElevationGain = &v
	}
	if validU16(s.TotalDescent) {
		v := int(s.TotalDescent)
		ss.ElevationLoss = &v
	}
	if validU32(s.TotalCalories) {
		v := int(s.TotalCalories)
		ss.Calories = &v
	}
	return ss
}

func buildWorkout(wkts []*mesgdef.Workout, steps []*mesgdef.WorkoutStep) *WorkoutInfo {
	if len(wkts) == 0 {
		return nil
	}
	w := wkts[0]
	wi := &WorkoutInfo{
		Name:  w.WktName,
		Sport: w.Sport.String(),
	}
	for _, ws := range steps {
		wi.Steps = append(wi.Steps, WorkoutStep{
			Name:          ws.WktStepName,
			DurationType:  ws.DurationType.String(),
			DurationValue: ws.DurationValue,
			TargetType:    ws.TargetType.String(),
			TargetValue:   ws.TargetValue,
			Notes:         ws.Notes,
		})
	}
	return wi
}

// ---- Open-Meteo elevation ----

// fetchOpenMeteoElevation fetches terrain elevations for a batch of coordinates.
// Splits into chunks of 100 to respect the API limit.
func fetchOpenMeteoElevation(lats, lons []float64) ([]float64, error) {
	const batchSize = 100
	client := &http.Client{Timeout: 10 * time.Second}
	result := make([]float64, 0, len(lats))

	for start := 0; start < len(lats); start += batchSize {
		end := start + batchSize
		if end > len(lats) {
			end = len(lats)
		}
		bLats := lats[start:end]
		bLons := lons[start:end]

		latStrs := make([]string, len(bLats))
		lonStrs := make([]string, len(bLons))
		for i, v := range bLats {
			latStrs[i] = fmt.Sprintf("%.6f", v)
		}
		for i, v := range bLons {
			lonStrs[i] = fmt.Sprintf("%.6f", v)
		}
		url := "https://api.open-meteo.com/v1/elevation?latitude=" +
			strings.Join(latStrs, ",") + "&longitude=" + strings.Join(lonStrs, ",")

		resp, err := client.Get(url)
		if err != nil {
			return nil, fmt.Errorf("open-meteo elevation request: %w", err)
		}
		var body struct {
			Elevation []float64 `json:"elevation"`
			Error     bool      `json:"error"`
			Reason    string    `json:"reason"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("open-meteo elevation decode: %w", err)
		}
		if body.Error {
			return nil, fmt.Errorf("open-meteo elevation API error: %s", body.Reason)
		}
		if len(body.Elevation) != len(bLats) {
			return nil, fmt.Errorf("open-meteo elevation: got %d values for %d points", len(body.Elevation), len(bLats))
		}
		result = append(result, body.Elevation...)
	}
	return result, nil
}

// smoothTriangle applies one pass of a 5-point triangular weighted moving
// average. Call multiple times for stronger smoothing.
func smoothTriangle(alts []float64) []float64 {
	weights := []float64{1, 2, 3, 2, 1}
	out := make([]float64, len(alts))
	for i := range alts {
		var s, w float64
		for j, wj := range weights {
			idx := i + j - 2
			if idx < 0 {
				idx = 0
			} else if idx >= len(alts) {
				idx = len(alts) - 1
			}
			s += alts[idx] * wj
			w += wj
		}
		out[i] = s / w
	}
	return out
}

// computeAscentDescent sums elevation gains and losses from a sequence of altitudes.
// Three passes of a 5-point triangular smoother (~36 m effective window at 1 Hz
// GPS) are applied first to suppress noise before differencing.
func computeAscentDescent(alts []float64) (ascent, descent int) {
	if len(alts) < 2 {
		return
	}

	smoothed := alts
	for range 30 {
		smoothed = smoothTriangle(smoothed)
	}

	var totalAscent, totalDescent float64
	for i := 1; i < len(smoothed); i++ {
		d := smoothed[i] - smoothed[i-1]
		if d > 0 {
			totalAscent += d
		} else if d < 0 {
			totalDescent -= d
		}
	}
	ascent = int(math.Round(totalAscent))
	descent = int(math.Round(totalDescent))
	return
}

// naturalCubicSpline fits a natural cubic spline through the given (x, y) knots
// (x must be strictly increasing) and returns a function that evaluates the
// spline at any x in [x[0], x[n-1]]. Values outside the range are clamped to
// the nearest endpoint.
func naturalCubicSpline(xs, ys []float64) func(float64) float64 {
	n := len(xs)
	if n < 2 {
		if n == 1 {
			v := ys[0]
			return func(float64) float64 { return v }
		}
		return func(float64) float64 { return 0 }
	}

	// Thomas algorithm for natural spline (second derivative = 0 at endpoints)
	h := make([]float64, n-1)
	for i := range h {
		h[i] = xs[i+1] - xs[i]
	}

	// set up tridiagonal system for second derivatives (sigma)
	size := n - 2
	sigma := make([]float64, n) // sigma[0] = sigma[n-1] = 0
	if size > 0 {
		diag := make([]float64, size)
		rhs := make([]float64, size)
		for i := 0; i < size; i++ {
			diag[i] = 2 * (h[i] + h[i+1])
			rhs[i] = 6 * ((ys[i+2]-ys[i+1])/h[i+1] - (ys[i+1]-ys[i])/h[i])
		}
		// forward elimination
		upper := make([]float64, size-1)
		for i := 0; i < size-1; i++ {
			upper[i] = h[i+1]
		}
		for i := 1; i < size; i++ {
			m := h[i] / diag[i-1]
			diag[i] -= m * upper[i-1]
			rhs[i] -= m * rhs[i-1]
		}
		// back substitution
		tmp := make([]float64, size)
		tmp[size-1] = rhs[size-1] / diag[size-1]
		for i := size - 2; i >= 0; i-- {
			tmp[i] = (rhs[i] - upper[i]*tmp[i+1]) / diag[i]
		}
		for i := 0; i < size; i++ {
			sigma[i+1] = tmp[i]
		}
	}

	return func(x float64) float64 {
		if x <= xs[0] {
			return ys[0]
		}
		if x >= xs[n-1] {
			return ys[n-1]
		}
		// binary search for interval
		lo, hi := 0, n-2
		for lo < hi {
			mid := (lo + hi) / 2
			if xs[mid+1] < x {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		i := lo
		dx := xs[i+1] - xs[i]
		t := x - xs[i]
		a := (xs[i+1] - x) / dx
		b := t / dx
		return a*ys[i] + b*ys[i+1] +
			((a*a*a-a)*sigma[i]+(b*b*b-b)*sigma[i+1])*(dx*dx)/6
	}
}



// buildHeightProfile samples altitude adaptively from raw records.
// If apiAltsByDist is non-nil it maps distance_m → terrain elevation from an
// external API; the nearest available API value replaces the FIT altitude for
// each sampled point (linear search over the small sorted key set is fine).
// A new point is emitted when:
//   - at least minDistM metres have passed since the last point AND
//     the altitude has changed by at least steepThreshM metres (steep section), OR
//   - maxDistM metres have passed since the last point (flat keep-alive).
//
// The final record is always emitted.
func buildHeightProfile(records []*mesgdef.Record, apiAltsByDist map[float64]float64) *HeightProfile {
	// collect valid (distance, altitude) pairs
	type raw struct{ dist, alt float64 }
	var pts []raw
	for _, r := range records {
		dist := optDist32(r.Distance)
		if dist == nil {
			continue
		}
		var alt *float64
		if apiAltsByDist != nil {
			// Find the nearest GPS track point by distance
			best := math.MaxFloat64
			var bestAlt float64
			for distKey, a := range apiAltsByDist {
				if d := math.Abs(*dist - distKey); d < best {
					best = d
					bestAlt = a
				}
			}
			alt = &bestAlt
		}
		if alt == nil {
			alt = optAlt32(r.EnhancedAltitude)
		}
		if alt == nil {
			alt = optAlt16(r.Altitude)
		}
		if alt == nil {
			continue
		}
		pts = append(pts, raw{*dist, *alt})
	}
	if len(pts) == 0 {
		return nil
	}

	// Pass 1: collect knots by grouping consecutive points into runs while
	// cumulative altitude change stays within ±1 m. Each run becomes one knot
	// placed at the run's midpoint distance with its average altitude.
	// This way the spline node sits in the middle of each terrain level and
	// slopes naturally into the next, instead of snapping at the transition edge.
	maxDistM := pts[len(pts)-1].dist
	var knots []raw
	runStart := 0
	for i := 1; i <= len(pts); i++ {
		endOfData := i == len(pts)
		crossed := !endOfData && math.Abs(pts[i].alt-pts[runStart].alt) > 1.0
		if crossed || endOfData {
			// average dist and alt over the run
			var sumDist, sumAlt float64
			for k := runStart; k < i; k++ {
				sumDist += pts[k].dist
				sumAlt += pts[k].alt
			}
			n := float64(i - runStart)
			knots = append(knots, raw{sumDist / n, sumAlt / n})
			runStart = i
		}
	}

	kx := make([]float64, len(knots))
	ky := make([]float64, len(knots))
	for i, k := range knots {
		kx[i] = k.dist
		ky[i] = k.alt
	}

	// Pass 2: spline through the knots, sample at 50 m intervals
	spline := naturalCubicSpline(kx, ky)

	// sample spline at uniform 50 m intervals
	var sampled []HeightProfilePoint
	for d := 0.0; d <= maxDistM+0.5; d += 50.0 {
		if d > maxDistM {
			d = maxDistM
		}
		sampled = append(sampled, HeightProfilePoint{
			DistanceKm: round3(d / 1000.0),
			AltitudeM:  math.Round(spline(d)*10) / 10,
		})
		if d == maxDistM {
			break
		}
	}

	// per-km gain/loss from the spline-sampled series
	type kmBucket struct{ gain, loss float64 }
	buckets := map[int]*kmBucket{}
	for i := 1; i < len(sampled); i++ {
		km := int(sampled[i].DistanceKm)
		d := sampled[i].AltitudeM - sampled[i-1].AltitudeM
		b := buckets[km]
		if b == nil {
			b = &kmBucket{}
			buckets[km] = b
		}
		if d > 0 {
			b.gain += d
		} else {
			b.loss -= d
		}
	}
	var kmElevation []KmElevation
	for km := 0; ; km++ {
		b, ok := buckets[km]
		if !ok {
			if km > int(sampled[len(sampled)-1].DistanceKm) {
				break
			}
			kmElevation = append(kmElevation, KmElevation{KmMark: km + 1})
			continue
		}
		kmElevation = append(kmElevation, KmElevation{
			KmMark: km + 1,
			GainM:  int(math.Round(b.gain)),
			LossM:  int(math.Round(b.loss)),
		})
	}

	// min/max and sparkline from smoothed sampled points
	minAlt := sampled[0].AltitudeM
	maxAlt := sampled[0].AltitudeM
	for _, sp := range sampled {
		if sp.AltitudeM < minAlt {
			minAlt = sp.AltitudeM
		}
		if sp.AltitudeM > maxAlt {
			maxAlt = sp.AltitudeM
		}
	}

	blocks := []rune("▁▂▃▄▅▆▇█")
	spark := make([]rune, len(sampled))
	rng := maxAlt - minAlt
	for i, sp := range sampled {
		idx := 0
		if rng > 0 {
			idx = int(math.Round((sp.AltitudeM - minAlt) / rng * float64(len(blocks)-1)))
		}
		spark[i] = blocks[idx]
	}

	return &HeightProfile{
		MinAltitudeM: math.Round(minAlt*10) / 10,
		MaxAltitudeM: math.Round(maxAlt*10) / 10,
		Points:       sampled,
		Sparkline:    string(spark),
		KmElevation:  kmElevation,
	}
}

// ---- main ----

func buildGpsTrack(records []*mesgdef.Record, intervalSec int, intervalDistM int) *GpsTrack {
	if intervalSec <= 0 && intervalDistM <= 0 {
		return nil
	}

	var points []GpsPoint
	var startTime time.Time
	lastEmittedSec := -intervalSec
	lastEmittedDistM := -float64(intervalDistM)

	for _, r := range records {
		lat := r.PositionLatDegrees()
		lon := r.PositionLongDegrees()
		if math.IsNaN(lat) || math.IsNaN(lon) {
			continue
		}
		dist := optDist32(r.Distance)
		if dist == nil {
			continue
		}

		if startTime.IsZero() {
			startTime = r.Timestamp
		}
		elapsed := int(r.Timestamp.Sub(startTime).Seconds())

		var emit bool
		if intervalDistM > 0 {
			emit = *dist-lastEmittedDistM >= float64(intervalDistM)
		} else {
			emit = elapsed-lastEmittedSec >= intervalSec
		}

		if emit {
			pt := GpsPoint{
				ElapsedSec: elapsed,
				DistanceKm: round3(*dist / 1000.0),
				Lat:        math.Round(lat*1e6) / 1e6,
				Lon:        math.Round(lon*1e6) / 1e6,
			}
			// attach FIT altitude if available
			alt := optAlt32(r.EnhancedAltitude)
			if alt == nil {
				alt = optAlt16(r.Altitude)
			}
			if alt != nil {
				v := math.Round(*alt*10) / 10
				pt.AltitudeFitM = &v
			}
			points = append(points, pt)
			lastEmittedSec = elapsed
			lastEmittedDistM = *dist
		}
	}

	if len(points) == 0 {
		return nil
	}
	track := &GpsTrack{Points: points}
	if intervalDistM > 0 {
		track.IntervalM = intervalDistM
	} else {
		track.IntervalSec = intervalSec
	}
	return track
}

func analyzeFile(path string, gpsIntervalSec int, gpsDistIntervalM int, elevationSource string) (Output, error) {
	f, err := os.Open(path)
	if err != nil {
		return Output{}, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	dec := decoder.New(f)
	rawFit, err := dec.Decode()
	if err != nil {
		return Output{}, fmt.Errorf("decoding fit: %w", err)
	}

	activity := filedef.NewActivity(rawFit.Messages...)

	if len(activity.Sessions) == 0 {
		return Output{}, fmt.Errorf("no session found in file")
	}

	sess := activity.Sessions[0]

	laps := make([]LapStats, 0, len(activity.Laps))
	for i, lap := range activity.Laps {
		laps = append(laps, buildLapStats(i+1, lap))
	}

	splits := make([]SplitStats, 0, len(activity.Splits))
	for i, s := range activity.Splits {
		splits = append(splits, buildSplitStats(i+1, s))
	}

	summaries := make([]SplitSummary, 0, len(activity.SplitSummaries))
	for _, s := range activity.SplitSummaries {
		summaries = append(summaries, buildSplitSummary(s))
	}

	gpsTrack := buildGpsTrack(activity.Records, gpsIntervalSec, gpsDistIntervalM)
	meta := buildMetadata(sess)

	// Fetch Open-Meteo elevations for the GPS track points and use them for
	// the height profile and ascent/descent computation.
	var apiAltsByDist map[float64]float64 // dist_m → api altitude
	if elevationSource == "open-meteo" && gpsTrack != nil && len(gpsTrack.Points) > 0 {
		lats := make([]float64, len(gpsTrack.Points))
		lons := make([]float64, len(gpsTrack.Points))
		for i, pt := range gpsTrack.Points {
			lats[i] = pt.Lat
			lons[i] = pt.Lon
		}
		apiAlts, err := fetchOpenMeteoElevation(lats, lons)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: open-meteo elevation fetch failed (%v), falling back to FIT data\n", err)
		} else {
			// Attach API altitudes to GPS track points
			apiAltsByDist = make(map[float64]float64, len(gpsTrack.Points))
			for i := range gpsTrack.Points {
				rounded := math.Round(apiAlts[i]*10) / 10
				gpsTrack.Points[i].AltitudeApiM = &rounded
				apiAltsByDist[gpsTrack.Points[i].DistanceKm*1000] = apiAlts[i]
			}

			// Recompute total ascent/descent from API altitudes
			asc, desc := computeAscentDescent(apiAlts)
			meta.TotalAscent = &asc
			meta.TotalDescent = &desc
		}
	}

	return Output{
		UserProfile:     buildUserProfile(activity.UserProfile),
		Metadata:        meta,
		RunningDynamics: buildSessionDynamics(sess),
		Workout:         buildWorkout(activity.Workouts, activity.WorkoutSteps),
		Laps:            laps,
		Splits:          splits,
		SplitSummaries:  summaries,
		HeightProfile:   buildHeightProfile(activity.Records, apiAltsByDist),
		GpsTrack:        gpsTrack,
	}, nil
}

// ---- SVG rendering ----

func renderHeightProfileSVG(hp *HeightProfile) string {
	const (
		svgW    = 800
		svgH    = 200
		padLeft = 60
		padRight  = 20
		padTop    = 20
		padBottom = 40
	)
	plotW := float64(svgW - padLeft - padRight)
	plotH := float64(svgH - padTop - padBottom)
	plotBottom := float64(svgH - padBottom)

	pts := hp.Points
	if len(pts) == 0 {
		return ""
	}

	maxDist := pts[len(pts)-1].DistanceKm
	minAlt := hp.MinAltitudeM
	maxAlt := hp.MaxAltitudeM
	// ensure the Y axis spans at least 30 m so flat runs don't look hilly
	const minAltSpanM = 30.0
	if maxAlt-minAlt < minAltSpanM {
		mid := (minAlt + maxAlt) / 2
		minAlt = mid - minAltSpanM/2
		maxAlt = mid + minAltSpanM/2
	}
	altRange := maxAlt - minAlt

	xScale := func(km float64) float64 {
		if maxDist == 0 {
			return float64(padLeft)
		}
		return float64(padLeft) + km/maxDist*plotW
	}
	yScale := func(alt float64) float64 {
		return float64(padTop) + (1-(alt-minAlt)/altRange)*plotH
	}

	// convert points to screen coordinates
	type pt struct{ x, y float64 }
	spts := make([]pt, len(pts))
	for i, p := range pts {
		spts[i] = pt{xScale(p.DistanceKm), yScale(p.AltitudeM)}
	}

	// catmullRomPath builds a smooth SVG path through the screen points using
	// Catmull-Rom converted to cubic bezier control points (tension = 0.5).
	catmullRomPath := func(points []pt) string {
		if len(points) < 2 {
			return ""
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "M %.2f,%.2f", points[0].x, points[0].y)
		for i := 0; i < len(points)-1; i++ {
			p0 := points[max(i-1, 0)]
			p1 := points[i]
			p2 := points[i+1]
			p3 := points[min(i+2, len(points)-1)]
			cp1x := p1.x + (p2.x-p0.x)/6
			cp1y := p1.y + (p2.y-p0.y)/6
			cp2x := p2.x - (p3.x-p1.x)/6
			cp2y := p2.y - (p3.y-p1.y)/6
			fmt.Fprintf(&sb, " C %.2f,%.2f %.2f,%.2f %.2f,%.2f",
				cp1x, cp1y, cp2x, cp2y, p2.x, p2.y)
		}
		return sb.String()
	}

	// filled area path: drop to bottom, trace curve, close
	linePath := catmullRomPath(spts)
	var sb strings.Builder
	fmt.Fprintf(&sb, "M %.2f,%.2f ", spts[0].x, plotBottom)
	fmt.Fprintf(&sb, "L %.2f,%.2f ", spts[0].x, spts[0].y)
	sb.WriteString(linePath[len(fmt.Sprintf("M %.2f,%.2f", spts[0].x, spts[0].y)):])
	fmt.Fprintf(&sb, " L %.2f,%.2f Z", spts[len(spts)-1].x, plotBottom)
	areaPath := sb.String()

	// nice Y-axis ticks
	niceStep := func(rng float64) float64 {
		for _, s := range []float64{1, 2, 5, 10, 25, 50, 100, 200, 500} {
			if rng/s <= 5 {
				return s
			}
		}
		return math.Ceil(rng / 5)
	}
	step := niceStep(altRange)
	firstTick := math.Ceil(minAlt/step) * step
	var yTicks []float64
	for t := firstTick; t <= maxAlt+step*0.01; t += step {
		yTicks = append(yTicks, t)
	}

	// X-axis km ticks
	totalKm := int(math.Ceil(maxDist))
	labelEvery := 1
	if totalKm > 20 {
		labelEvery = 5
	} else if totalKm > 9 {
		labelEvery = 2
	}

	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`+"\n", svgW, svgH, svgW, svgH)

	// gradient
	out.WriteString(`  <defs>` + "\n")
	out.WriteString(`    <linearGradient id="hpGrad" x1="0" y1="0" x2="0" y2="1">` + "\n")
	out.WriteString(`      <stop offset="0%" stop-color="#4a90d9" stop-opacity="0.7"/>` + "\n")
	out.WriteString(`      <stop offset="100%" stop-color="#4a90d9" stop-opacity="0.1"/>` + "\n")
	out.WriteString(`    </linearGradient>` + "\n")
	out.WriteString(`  </defs>` + "\n")

	// background
	fmt.Fprintf(&out, `  <rect width="%d" height="%d" fill="#f8f9fa"/>`, svgW, svgH)
	out.WriteString("\n")

	// horizontal grid lines at Y ticks
	for _, t := range yTicks {
		y := yScale(t)
		if y < float64(padTop) || y > plotBottom {
			continue
		}
		fmt.Fprintf(&out, `  <line x1="%d" y1="%.2f" x2="%d" y2="%.2f" stroke="#ddd" stroke-width="1"/>`,
			padLeft, y, svgW-padRight, y)
		out.WriteString("\n")
	}

	// filled area
	fmt.Fprintf(&out, `  <path d="%s" fill="url(#hpGrad)"/>`, areaPath)
	out.WriteString("\n")

	// stroke line
	fmt.Fprintf(&out, `  <path d="%s" fill="none" stroke="#2171b5" stroke-width="1.5" stroke-linejoin="round"/>`, linePath)
	out.WriteString("\n")

	// axes
	fmt.Fprintf(&out, `  <line x1="%d" y1="%.2f" x2="%d" y2="%.2f" stroke="#888" stroke-width="1"/>`,
		padLeft, float64(padTop), padLeft, plotBottom)
	out.WriteString("\n")
	fmt.Fprintf(&out, `  <line x1="%d" y1="%.2f" x2="%d" y2="%.2f" stroke="#888" stroke-width="1"/>`,
		padLeft, plotBottom, svgW-padRight, plotBottom)
	out.WriteString("\n")

	// Y-axis labels
	for _, t := range yTicks {
		y := yScale(t)
		if y < float64(padTop) || y > plotBottom {
			continue
		}
		fmt.Fprintf(&out, `  <text x="%d" y="%.2f" text-anchor="end" font-family="sans-serif" font-size="10" fill="#555">%dm</text>`,
			padLeft-4, y+3.5, int(t))
		out.WriteString("\n")
	}

	// X-axis km ticks and labels
	for km := 0; km <= totalKm; km++ {
		x := xScale(float64(km))
		fmt.Fprintf(&out, `  <line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="#888" stroke-width="1"/>`,
			x, plotBottom, x, plotBottom+4)
		out.WriteString("\n")
		if km%labelEvery == 0 {
			fmt.Fprintf(&out, `  <text x="%.2f" y="%.2f" text-anchor="middle" font-family="sans-serif" font-size="10" fill="#555">%dkm</text>`,
				x, plotBottom+15, km)
			out.WriteString("\n")
		}
	}

	out.WriteString("</svg>\n")
	return out.String()
}

func main() {
	outputFmt := flag.String("format", "yaml", "output format: yaml or json")
	svgDir := flag.String("svg-dir", "", "directory to write SVG height profiles (default: next to .fit file)")
	gpsInterval := flag.Int("gps-interval", 60, "GPS track sampling interval in seconds (0 to disable; use --gps-dist-interval for distance-based sampling)")
	gpsDistInterval := flag.Int("gps-dist-interval", 0, "GPS track sampling interval in metres (overrides --gps-interval when > 0)")
	elevationSource := flag.String("elevation-source", "open-meteo", "elevation source: open-meteo or fit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: fit-analyzer [--format yaml|json] [--svg-dir DIR] [--gps-interval N] <file.fit> [file2.fit ...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	exitCode := 0
	outputs := make([]Output, 0, flag.NArg())
	for _, fitPath := range flag.Args() {
		out, err := analyzeFile(fitPath, *gpsInterval, *gpsDistInterval, *elevationSource)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", fitPath, err)
			exitCode = 1
			continue
		}
		if out.HeightProfile != nil {
			svg := renderHeightProfileSVG(out.HeightProfile)
			if svg != "" {
				base := strings.TrimSuffix(filepath.Base(fitPath), filepath.Ext(fitPath)) + ".svg"
				dir := filepath.Dir(fitPath)
				if *svgDir != "" {
					dir = *svgDir
				}
				svgPath := filepath.Join(dir, base)
				if werr := os.WriteFile(svgPath, []byte(svg), 0644); werr != nil {
					fmt.Fprintf(os.Stderr, "WARNING: could not write SVG %s: %v\n", svgPath, werr)
				} else {
					out.HeightProfile.SvgPath = svgPath
				}
			}
		}
		outputs = append(outputs, out)
	}

	if len(outputs) == 0 {
		os.Exit(exitCode)
	}

	var (
		data []byte
		err  error
	)
	if len(outputs) == 1 {
		switch *outputFmt {
		case "json":
			data, err = json.MarshalIndent(outputs[0], "", "  ")
		default:
			data, err = yaml.Marshal(outputs[0])
		}
	} else {
		switch *outputFmt {
		case "json":
			data, err = json.MarshalIndent(outputs, "", "  ")
		default:
			data, err = yaml.Marshal(outputs)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshalling output: %v\n", err)
		os.Exit(1)
	}

	os.Stdout.Write(data)
	os.Exit(exitCode)
}
