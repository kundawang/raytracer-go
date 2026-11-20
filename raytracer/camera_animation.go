package raytracer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
)

// CameraKeyframe describes the camera's pose at a point in time (in seconds).
//
// From is the position of the eye. (Point)
// To is the position the eye's looking at. (Point)
// Up specifies the direction of up. (Vector)
type CameraKeyframe struct {
	Time float64
	From Tuple
	To   Tuple
	Up   Tuple
}

// CameraPose is an interpolated camera pose at a single instant.
type CameraPose struct {
	From Tuple
	To   Tuple
	Up   Tuple
}

// CameraAnimation animates a Camera along a track of CameraKeyframes.
//
// Frames are sampled at FPS frames-per-second, starting at the first
// keyframe's time. Between keyframes the From/To/Up values are linearly
// interpolated (Up is re-normalized after interpolation), so the camera
// moves at a constant speed between keyframes.
//
// Keyframes are sorted by time when the animation is created, so they may be
// specified in any order. If multiple keyframes share the same time, the one
// specified last wins.
//
// If Loop is true, the animation's last frame is the frame *before* it would
// reach the final keyframe's time, and sampling at (or past) the end time
// wraps back to the start -- so an animation whose first and last keyframes
// have identical poses loops seamlessly. If Loop is false, the animation
// includes a final frame exactly at the last keyframe's time.
//
// A CameraAnimation is safe to use from a single goroutine at a time; each
// rendered frame gets its own copy of the base Camera, and rendering a frame
// is deterministic regardless of the number of parallel jobs used.
type CameraAnimation struct {
	Camera    *Camera
	Keyframes []CameraKeyframe
	FPS       float64
	Loop      bool
}

// NewCameraAnimation returns a CameraAnimation, or an error if the arguments
// are invalid (no keyframes, non-positive FPS, non-finite times, or
// degenerate keyframe poses).
func NewCameraAnimation(camera *Camera, fps float64, loop bool, keyframes []CameraKeyframe) (*CameraAnimation, error) {
	if camera == nil {
		return nil, errors.New("camera must not be nil")
	}
	if math.IsNaN(fps) || math.IsInf(fps, 0) || fps <= 0 {
		return nil, fmt.Errorf("fps must be a positive, finite number, got %v", fps)
	}
	if len(keyframes) == 0 {
		return nil, errors.New("at least one keyframe is required")
	}
	for idx, kf := range keyframes {
		if math.IsNaN(kf.Time) || math.IsInf(kf.Time, 0) {
			return nil, fmt.Errorf("keyframe %d has a non-finite time: %v", idx, kf.Time)
		}
		if err := validateCameraPose(kf.From, kf.To, kf.Up); err != nil {
			return nil, fmt.Errorf("keyframe %d (time %v): %s", idx, kf.Time, err)
		}
	}

	// Sort by time (stable, so input order is preserved for equal times) ...
	sorted := make([]CameraKeyframe, len(keyframes))
	copy(sorted, keyframes)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Time < sorted[j].Time })

	// ... and collapse duplicate timestamps, keeping the last one specified.
	deduped := []CameraKeyframe{sorted[0]}
	for _, kf := range sorted[1:] {
		if deduped[len(deduped)-1].Time == kf.Time {
			deduped[len(deduped)-1] = kf
		} else {
			deduped = append(deduped, kf)
		}
	}

	return &CameraAnimation{
		Camera:    camera,
		Keyframes: deduped,
		FPS:       fps,
		Loop:      loop,
	}, nil
}

// StartTime returns the time of the first keyframe.
func (a *CameraAnimation) StartTime() float64 {
	return a.Keyframes[0].Time
}

// EndTime returns the time of the last keyframe.
func (a *CameraAnimation) EndTime() float64 {
	return a.Keyframes[len(a.Keyframes)-1].Time
}

// Duration returns the length of the animation in seconds. A single keyframe
// (or keyframes that all share one timestamp) yields a zero duration.
func (a *CameraAnimation) Duration() float64 {
	return a.EndTime() - a.StartTime()
}

// FrameCount returns the number of frames in the animation. It is always at
// least 1, even for a zero-duration animation.
func (a *CameraAnimation) FrameCount() int {
	steps := int(math.Round(a.Duration() * a.FPS))
	if steps < 1 {
		return 1
	}
	if a.Loop {
		return steps
	}
	return steps + 1
}

// FrameTime returns the time (in seconds) at which the given frame is sampled.
func (a *CameraAnimation) FrameTime(frame int) (float64, error) {
	if frame < 0 || frame >= a.FrameCount() {
		return 0, fmt.Errorf("frame %d is out of range [0, %d)", frame, a.FrameCount())
	}
	// Make sure the final frame of a non-looping animation lands exactly on
	// the last keyframe, without floating-point drift.
	if !a.Loop && frame == a.FrameCount()-1 {
		return a.EndTime(), nil
	}
	return a.StartTime() + float64(frame)/a.FPS, nil
}

// PoseAt returns the interpolated camera pose at time t (in seconds).
//
// For looping animations, t wraps around into [StartTime, EndTime). For
// non-looping animations, t is clamped to [StartTime, EndTime].
func (a *CameraAnimation) PoseAt(t float64) (CameraPose, error) {
	if math.IsNaN(t) || math.IsInf(t, 0) {
		return CameraPose{}, fmt.Errorf("time must be finite, got %v", t)
	}

	first := a.Keyframes[0]
	last := a.Keyframes[len(a.Keyframes)-1]
	duration := a.Duration()

	if duration == 0 {
		return CameraPose{first.From, first.To, first.Up}, nil
	}

	if a.Loop {
		t = first.Time + altMod(t-first.Time, duration)
	} else if t < first.Time {
		t = first.Time
	} else if t > last.Time {
		t = last.Time
	}

	// Find the keyframe segment containing t.
	idx := sort.Search(len(a.Keyframes), func(i int) bool { return a.Keyframes[i].Time > t }) - 1
	if idx < 0 {
		return CameraPose{first.From, first.To, first.Up}, nil
	}
	if idx >= len(a.Keyframes)-1 {
		return CameraPose{last.From, last.To, last.Up}, nil
	}

	kf0 := a.Keyframes[idx]
	kf1 := a.Keyframes[idx+1]
	alpha := (t - kf0.Time) / (kf1.Time - kf0.Time)

	pose := CameraPose{
		From: lerpTuple(kf0.From, kf1.From, alpha),
		To:   lerpTuple(kf0.To, kf1.To, alpha),
		Up:   lerpTuple(kf0.Up, kf1.Up, alpha).Normalized(),
	}
	if err := validateCameraPose(pose.From, pose.To, pose.Up); err != nil {
		return CameraPose{}, fmt.Errorf("interpolated pose at time %v: %s", t, err)
	}
	return pose, nil
}

// PoseForFrame returns the camera pose sampled for the given frame.
func (a *CameraAnimation) PoseForFrame(frame int) (CameraPose, error) {
	t, err := a.FrameTime(frame)
	if err != nil {
		return CameraPose{}, err
	}
	return a.PoseAt(t)
}

// CameraForFrame returns a copy of the animation's base Camera, oriented for
// the given frame. The animation's own Camera is never modified.
func (a *CameraAnimation) CameraForFrame(frame int) (*Camera, error) {
	pose, err := a.PoseForFrame(frame)
	if err != nil {
		return nil, err
	}
	camera := *a.Camera
	camera.SetTransform(NewViewTransform(pose.From, pose.To, pose.Up))
	return &camera, nil
}

// RenderFrame renders a single frame of the animation and returns its canvas.
func (a *CameraAnimation) RenderFrame(w *World, frame int, jobs int, printProgress bool) (Canvas, error) {
	camera, err := a.CameraForFrame(frame)
	if err != nil {
		return Canvas{}, err
	}
	return camera.Render(w, jobs, printProgress), nil
}

// Render renders every frame of the animation in order, invoking onFrame with
// each frame's canvas (e.g. to save it to a file). Rendering stops early if
// ctx is canceled or onFrame returns an error. The world is only read, never
// modified, and each frame is deterministic regardless of the jobs count.
func (a *CameraAnimation) Render(ctx context.Context, w *World, jobs int, printProgress bool, onFrame func(frame int, canvas Canvas) error) error {
	for frame := 0; frame < a.FrameCount(); frame++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("animation canceled before frame %d: %w", frame, err)
		}
		canvas, err := a.RenderFrame(w, frame, jobs, printProgress)
		if err != nil {
			return err
		}
		if onFrame != nil {
			if err := onFrame(frame, canvas); err != nil {
				return fmt.Errorf("animation stopped at frame %d: %w", frame, err)
			}
		}
	}
	return nil
}

// lerpTuple linearly interpolates between two tuples of the same type.
func lerpTuple(t0, t1 Tuple, alpha float64) Tuple {
	return t0.Add(t1.Subtract(t0).Multiply(alpha))
}

// validateCameraPose returns an error if the pose would produce a degenerate
// view transform (e.g. NaNs): the eye sitting on its target, a zero up
// vector, or an up vector parallel to the view direction.
//
// NB: the magnitude checks use !(x >= EPSILON) rather than x < EPSILON so
// that NaN magnitudes are also rejected.
func validateCameraPose(from, to, up Tuple) error {
	if !isFiniteTuple(from) || !isFiniteTuple(to) || !isFiniteTuple(up) {
		return errors.New("pose contains non-finite values")
	}
	if from.W != 1.0 || to.W != 1.0 {
		return fmt.Errorf("from and to must be points, got %s and %s", from.Type(), to.Type())
	}
	if up.W != 0.0 {
		return fmt.Errorf("up must be a vector, got %s", up.Type())
	}
	if from.IsEqualTo(to) {
		return fmt.Errorf("degenerate pose: from %v is the same as to %v", from, to)
	}
	if !(up.Magnitude() >= EPSILON) {
		return fmt.Errorf("degenerate pose: up vector %v is zero", up)
	}
	forward := to.Subtract(from).Normalized()
	if !(forward.Cross(up.Normalized()).Magnitude() >= EPSILON) {
		return fmt.Errorf("degenerate pose: up vector %v is parallel to the view direction", up)
	}
	return nil
}

func isFiniteTuple(t Tuple) bool {
	return !math.IsNaN(t.X) && !math.IsNaN(t.Y) && !math.IsNaN(t.Z) && !math.IsNaN(t.W) &&
		!math.IsInf(t.X, 0) && !math.IsInf(t.Y, 0) && !math.IsInf(t.Z, 0) && !math.IsInf(t.W, 0)
}
