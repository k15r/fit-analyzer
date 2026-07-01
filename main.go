package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
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
	ElapsedSec int     `json:"elapsed_sec" yaml:"elapsed_sec"`
	DistanceKm float64 `json:"distance_km" yaml:"distance_km"`
	Lat        float64 `json:"lat" yaml:"lat"`
	Lon        float64 `json:"lon" yaml:"lon"`
}

type GpsTrack struct {
	IntervalSec int        `json:"interval_sec" yaml:"interval_sec"`
	Points      []GpsPoint `json:"points" yaml:"points"`
}

type HeightProfilePoint struct {
	DistanceKm float64 `json:"distance_km" yaml:"distance_km"`
	AltitudeM  float64 `json:"altitude_m" yaml:"altitude_m"`
}

type HeightProfile struct {
	MinAltitudeM float64              `json:"min_altitude_m" yaml:"min_altitude_m"`
	MaxAltitudeM float64              `json:"max_altitude_m" yaml:"max_altitude_m"`
	Points       []HeightProfilePoint `json:"points" yaml:"points"`
	Sparkline    string               `json:"sparkline" yaml:"sparkline"`
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

// ---- height profile ----

// buildHeightProfile samples altitude adaptively from raw records.
// A new point is emitted when:
//   - at least minDistM metres have passed since the last point AND
//     the altitude has changed by at least steepThreshM metres (steep section), OR
//   - maxDistM metres have passed since the last point (flat keep-alive).
//
// The final record is always emitted.
func buildHeightProfile(records []*mesgdef.Record) *HeightProfile {
	const (
		minDistM     = 50.0  // minimum spacing between any two points
		maxDistM     = 1000.0 // maximum spacing on flat terrain
		steepThreshM = 2.0  // altitude change that triggers early emission
	)

	// collect valid (distance, altitude) pairs
	type raw struct{ dist, alt float64 }
	var pts []raw
	for _, r := range records {
		alt := optAlt32(r.EnhancedAltitude)
		if alt == nil {
			alt = optAlt16(r.Altitude)
		}
		dist := optDist32(r.Distance)
		if alt == nil || dist == nil {
			continue
		}
		pts = append(pts, raw{*dist, *alt})
	}
	if len(pts) == 0 {
		return nil
	}

	// adaptive sampling
	var sampled []HeightProfilePoint
	lastDist := pts[0].dist
	lastAlt := pts[0].alt
	sampled = append(sampled, HeightProfilePoint{
		DistanceKm: round3(lastDist / 1000.0),
		AltitudeM:  math.Round(lastAlt*10) / 10,
	})

	for i := 1; i < len(pts); i++ {
		p := pts[i]
		dDist := p.dist - lastDist
		dAlt := math.Abs(p.alt - lastAlt)
		isLast := i == len(pts)-1

		emit := isLast ||
			(dDist >= minDistM && dAlt >= steepThreshM) ||
			dDist >= maxDistM

		if emit {
			sampled = append(sampled, HeightProfilePoint{
				DistanceKm: round3(p.dist / 1000.0),
				AltitudeM:  math.Round(p.alt*10) / 10,
			})
			lastDist = p.dist
			lastAlt = p.alt
		}
	}

	// compute min/max and sparkline
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
	}
}

// ---- main ----

func buildGpsTrack(records []*mesgdef.Record, intervalSec int) *GpsTrack {
	if intervalSec <= 0 {
		return nil
	}

	var points []GpsPoint
	var startTime time.Time
	lastEmittedSec := -intervalSec // emit first valid point immediately

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

		if elapsed-lastEmittedSec >= intervalSec {
			points = append(points, GpsPoint{
				ElapsedSec: elapsed,
				DistanceKm: round3(*dist / 1000.0),
				Lat:        math.Round(lat*1e6) / 1e6,
				Lon:        math.Round(lon*1e6) / 1e6,
			})
			lastEmittedSec = elapsed
		}
	}

	if len(points) == 0 {
		return nil
	}
	return &GpsTrack{
		IntervalSec: intervalSec,
		Points:      points,
	}
}

func analyzeFile(path string, gpsIntervalSec int) (Output, error) {
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

	return Output{
		UserProfile:     buildUserProfile(activity.UserProfile),
		Metadata:        buildMetadata(sess),
		RunningDynamics: buildSessionDynamics(sess),
		Workout:         buildWorkout(activity.Workouts, activity.WorkoutSteps),
		Laps:            laps,
		Splits:          splits,
		SplitSummaries:  summaries,
		HeightProfile:   buildHeightProfile(activity.Records),
		GpsTrack:        buildGpsTrack(activity.Records, gpsIntervalSec),
	}, nil
}

func main() {
	outputFmt := flag.String("format", "yaml", "output format: yaml or json")
	gpsInterval := flag.Int("gps-interval", 60, "GPS track sampling interval in seconds (0 to disable)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: fit-analyzer [--format yaml|json] [--gps-interval N] <file.fit> [file2.fit ...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	exitCode := 0
	outputs := make([]Output, 0, flag.NArg())
	for _, path := range flag.Args() {
		out, err := analyzeFile(path, *gpsInterval)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exitCode = 1
			continue
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
