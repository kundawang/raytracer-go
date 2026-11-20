package raytracer

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func threeKeyframes() []CameraKeyframe {
	return []CameraKeyframe{
		{Time: 0, From: NewPoint(0, 0, -5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
		{Time: 1, From: NewPoint(10, 0, -5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
		{Time: 2, From: NewPoint(10, 0, 5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
	}
}

func newTestAnimation(t *testing.T, loop bool) *CameraAnimation {
	t.Helper()
	a, err := NewCameraAnimation(NewCamera(20, 10, math.Pi/2), 2, loop, threeKeyframes())
	if err != nil {
		t.Fatalf("unexpected error creating animation: %s", err)
	}
	return a
}

func TestCameraAnimationRequiresKeyframes(t *testing.T) {
	_, err := NewCameraAnimation(NewCamera(1, 1, 1), 24, false, []CameraKeyframe{})
	assertEqualError(t, errors.New("at least one keyframe is required"), err)
}

func TestCameraAnimationRequiresValidFPS(t *testing.T) {
	for _, fps := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := NewCameraAnimation(NewCamera(1, 1, 1), fps, false, threeKeyframes()); err == nil {
			t.Errorf("expected error for fps %v, got nil", fps)
		}
	}
}

func TestCameraAnimationRequiresCamera(t *testing.T) {
	if _, err := NewCameraAnimation(nil, 24, false, threeKeyframes()); err == nil {
		t.Error("expected error for nil camera, got nil")
	}
}

func TestCameraAnimationSortsReversedKeyframes(t *testing.T) {
	kfs := threeKeyframes()
	reversed := []CameraKeyframe{kfs[2], kfs[1], kfs[0]}
	a, err := NewCameraAnimation(NewCamera(1, 1, 1), 24, false, reversed)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualFloat64(t, 0, a.StartTime())
	assertEqualFloat64(t, 2, a.EndTime())
	assertEqualFloat64(t, 2, a.Duration())

	pose, err := a.PoseAt(0.5)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(5, 0, -5), pose.From)
}

func TestCameraAnimationDuplicateTimesLastOneWins(t *testing.T) {
	kfs := []CameraKeyframe{
		{Time: 0, From: NewPoint(0, 0, -5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
		{Time: 1, From: NewPoint(99, 0, -5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
		{Time: 1, From: NewPoint(10, 0, -5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
		{Time: 2, From: NewPoint(10, 0, 5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
	}
	a, err := NewCameraAnimation(NewCamera(1, 1, 1), 24, false, kfs)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualInt(t, 3, len(a.Keyframes))

	pose, err := a.PoseAt(1)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(10, 0, -5), pose.From)
}

func TestCameraAnimationZeroDuration(t *testing.T) {
	kfs := []CameraKeyframe{
		{Time: 1, From: NewPoint(0, 0, -5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
		{Time: 1, From: NewPoint(0, 0, -5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
	}
	a, err := NewCameraAnimation(NewCamera(1, 1, 1), 24, false, kfs)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualFloat64(t, 0, a.Duration())
	assertEqualInt(t, 1, a.FrameCount())

	pose, err := a.PoseForFrame(0)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(0, 0, -5), pose.From)
}

func TestCameraAnimationDegeneratePoses(t *testing.T) {
	testCases := []struct {
		name    string
		from    Tuple
		to      Tuple
		up      Tuple
		message string
	}{
		{"from equals to", NewPoint(0, 0, 0), NewPoint(0, 0, 0), NewVector(0, 1, 0), "degenerate"},
		{"zero up vector", NewPoint(0, 0, -5), NewPoint(0, 0, 0), NewVector(0, 0, 0), "degenerate"},
		{"up parallel to view", NewPoint(0, -5, 0), NewPoint(0, 0, 0), NewVector(0, 1, 0), "degenerate"},
		{"from is not a point", NewVector(0, 0, -5), NewPoint(0, 0, 0), NewVector(0, 1, 0), "must be points"},
		{"up is not a vector", NewPoint(0, 0, -5), NewPoint(0, 0, 0), NewPoint(0, 1, 0), "must be a vector"},
		{"non-finite from", NewPoint(math.NaN(), 0, -5), NewPoint(0, 0, 0), NewVector(0, 1, 0), "non-finite"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kfs := []CameraKeyframe{{Time: 0, From: tc.from, To: tc.to, Up: tc.up}}
			_, err := NewCameraAnimation(NewCamera(1, 1, 1), 24, false, kfs)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.message)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Errorf("expected error containing %q, got %q", tc.message, err.Error())
			}
		})
	}
}

func TestCameraAnimationInterpolatedDegeneratePose(t *testing.T) {
	// The interpolated From passes through the To point at t=0.5.
	kfs := []CameraKeyframe{
		{Time: 0, From: NewPoint(-5, 0, 0), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
		{Time: 1, From: NewPoint(5, 0, 0), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
	}
	a, err := NewCameraAnimation(NewCamera(1, 1, 1), 24, false, kfs)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if _, err := a.PoseAt(0.5); err == nil {
		t.Error("expected error for interpolated degenerate pose, got nil")
	}
}

func TestCameraAnimationFrameCountAndTimes(t *testing.T) {
	a := newTestAnimation(t, false)
	// duration 2s at 2fps, non-looping: 4 steps + final frame
	assertEqualInt(t, 5, a.FrameCount())
	for frame, expected := range []float64{0, 0.5, 1, 1.5, 2} {
		actual, err := a.FrameTime(frame)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		assertEqualFloat64(t, expected, actual)
	}

	looping := newTestAnimation(t, true)
	// looping: the frame at the end time is omitted, since it equals frame 0
	assertEqualInt(t, 4, looping.FrameCount())
}

func TestCameraAnimationFrameOutOfRange(t *testing.T) {
	a := newTestAnimation(t, false)
	for _, frame := range []int{-1, 5, 100} {
		if _, err := a.FrameTime(frame); err == nil {
			t.Errorf("expected error for frame %d, got nil", frame)
		}
		if _, err := a.RenderFrame(DefaultWorld(), frame, 1, false); err == nil {
			t.Errorf("expected render error for frame %d, got nil", frame)
		}
	}
}

func TestCameraAnimationMiddlePose(t *testing.T) {
	a := newTestAnimation(t, false)

	// halfway between keyframes 0 and 1
	pose, err := a.PoseAt(0.5)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(5, 0, -5), pose.From)
	assertEqualTuple(t, NewPoint(0, 0, 0), pose.To)
	assertEqualTuple(t, NewVector(0, 1, 0), pose.Up)

	// halfway between keyframes 1 and 2, via the frame at t=1.5
	pose, err = a.PoseForFrame(3)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(10, 0, 0), pose.From)

	// exactly on the middle keyframe
	pose, err = a.PoseAt(1)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(10, 0, -5), pose.From)
}

func TestCameraAnimationClampsAndWrapsTime(t *testing.T) {
	a := newTestAnimation(t, false)

	// non-looping: times outside the range clamp to the endpoints
	pose, err := a.PoseAt(-10)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(0, 0, -5), pose.From)
	pose, err = a.PoseAt(10)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(10, 0, 5), pose.From)

	// looping: times wrap around, and the end time wraps to the start pose
	looping := newTestAnimation(t, true)
	pose, err = looping.PoseAt(2)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(0, 0, -5), pose.From)
	pose, err = looping.PoseAt(2.5)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(5, 0, -5), pose.From)
}

func TestCameraAnimationLoopSeam(t *testing.T) {
	// A closed orbit: the last keyframe repeats the first pose.
	kfs := []CameraKeyframe{
		{Time: 0, From: NewPoint(0, 1, -5), To: NewPoint(0, 1, 0), Up: NewVector(0, 1, 0)},
		{Time: 1, From: NewPoint(5, 1, 0), To: NewPoint(0, 1, 0), Up: NewVector(0, 1, 0)},
		{Time: 2, From: NewPoint(0, 1, 5), To: NewPoint(0, 1, 0), Up: NewVector(0, 1, 0)},
		{Time: 3, From: NewPoint(-5, 1, 0), To: NewPoint(0, 1, 0), Up: NewVector(0, 1, 0)},
		{Time: 4, From: NewPoint(0, 1, -5), To: NewPoint(0, 1, 0), Up: NewVector(0, 1, 0)},
	}
	a, err := NewCameraAnimation(NewCamera(1, 1, 1), 4, true, kfs)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// the pose at the seam (end time) equals the pose at the start
	start, err := a.PoseAt(a.StartTime())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	seam, err := a.PoseAt(a.EndTime())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, start.From, seam.From)
	assertEqualTuple(t, start.To, seam.To)
	assertEqualTuple(t, start.Up, seam.Up)
}

func TestCameraAnimationFirstAndLastFramePixels(t *testing.T) {
	world := DefaultWorld()
	a := newTestAnimation(t, false)

	first, err := a.RenderFrame(world, 0, 1, false)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	last, err := a.RenderFrame(world, a.FrameCount()-1, 1, false)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// the first/last frames must match a static render from the same poses
	kfs := threeKeyframes()
	for frame, expected := range map[int]Canvas{0: first, a.FrameCount() - 1: last} {
		staticCamera := NewCamera(20, 10, math.Pi/2)
		staticCamera.SetTransform(NewViewTransform(kfs[frame/2].From, kfs[frame/2].To, kfs[frame/2].Up))
		staticCanvas := staticCamera.Render(world, 1, false)
		for _, pixel := range [][2]int{{0, 0}, {10, 5}, {19, 9}} {
			x, y := pixel[0], pixel[1]
			assertEqualColor(t, staticCanvas.PixelAt(x, y), expected.PixelAt(x, y))
		}
	}
}

func TestCameraAnimationDeterministicAcrossJobCounts(t *testing.T) {
	world := DefaultWorld()
	// include an area light with a multi-value jitter sequence, to prove the
	// result doesn't depend on the order pixels happen to be rendered in
	seq := NewSequence(0.7, 0.3, 0.9, 0.1, 0.5)
	light := NewAreaLight(NewPoint(-0.5, -0.5, -5), NewVector(1, 0, 0), 2, NewVector(0, 1, 0), 2, NewColor(1, 1, 1))
	light.Jitter = &seq
	world.Lights = []*AreaLight{light}

	a := newTestAnimation(t, false)
	serial, err := a.RenderFrame(world, 1, 1, false)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	parallel, err := a.RenderFrame(world, 1, 8, false)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !serial.IsEqualTo(parallel) {
		t.Error("expected identical canvases for jobs=1 and jobs=8")
	}

	// rendering the same frame twice is also stable
	again, err := a.RenderFrame(world, 1, 4, false)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !serial.IsEqualTo(again) {
		t.Error("expected identical canvases when rendering the same frame twice")
	}
}

func TestCameraAnimationRenderVisitsEveryFrame(t *testing.T) {
	a := newTestAnimation(t, false)
	frames := []int{}
	err := a.Render(context.Background(), DefaultWorld(), 1, false, func(frame int, canvas Canvas) error {
		frames = append(frames, frame)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualInt(t, a.FrameCount(), len(frames))
	for i, frame := range frames {
		assertEqualInt(t, i, frame)
	}
}

func TestCameraAnimationRenderCancel(t *testing.T) {
	a := newTestAnimation(t, false)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := a.Render(ctx, DefaultWorld(), 1, false, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	// canceling mid-way stops before the remaining frames are rendered
	ctx, cancel = context.WithCancel(context.Background())
	rendered := 0
	err = a.Render(ctx, DefaultWorld(), 1, false, func(frame int, canvas Canvas) error {
		rendered += 1
		if rendered == 2 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	assertEqualInt(t, 2, rendered)
}

func TestCameraAnimationRenderStopsOnCallbackError(t *testing.T) {
	a := newTestAnimation(t, false)
	myErr := errors.New("disk is full")
	rendered := 0
	err := a.Render(context.Background(), DefaultWorld(), 1, false, func(frame int, canvas Canvas) error {
		rendered += 1
		return myErr
	})
	if !errors.Is(err, myErr) {
		t.Errorf("expected %v, got %v", myErr, err)
	}
	assertEqualInt(t, 1, rendered)
}

func TestCameraAnimationManyFrames(t *testing.T) {
	// a long animation: frame count and sampling stay well-defined, and the
	// render loop can be stopped early without rendering everything
	kfs := []CameraKeyframe{
		{Time: 0, From: NewPoint(0, 0, -5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
		{Time: 1000, From: NewPoint(10, 0, -5), To: NewPoint(0, 0, 0), Up: NewVector(0, 1, 0)},
	}
	a, err := NewCameraAnimation(NewCamera(1, 1, 1), 1000, false, kfs)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualInt(t, 1000001, a.FrameCount())

	// frame 500000 samples t=500, halfway between the two keyframes
	pose, err := a.PoseForFrame(500000)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(5, 0, -5), pose.From)

	// the final frame lands exactly on the last keyframe
	pose, err = a.PoseForFrame(a.FrameCount() - 1)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	assertEqualTuple(t, NewPoint(10, 0, -5), pose.From)
}
