// Phase 0: any の枝が無名アクション（all {...} の直書き）の場合の actionTaken
module probe4 {
  var x: int
  action init = x' = 0
  action named = x' = x + 1
  action step = any {
    named,
    all { x > 100, x' = 0 },
    x' = x + 10,
  }
}
