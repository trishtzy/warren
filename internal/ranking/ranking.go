package ranking

// DefaultGravity controls how quickly posts decay in the ranking.
// Higher values make posts fall off the front page faster.
// Uses HN-style formula: score / (age_hours + 2) ^ gravity
const DefaultGravity = 1.5
