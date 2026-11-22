// Phase 0: bigint の境界、入れ子の any、init の nondet、step が単一アクションの場合
module probe2 {
  var a: int
  var b: int
  var c: int
  var d: int
  var e: int
  var label: str

  action init = {
    nondet s = Set(1, 2).oneOf()
    all { a' = 10^15 - 1, b' = 10^15, c' = -(10^15), d' = -5, e' = s, label' = "init" }
  }
  action left = all { a' = a, b' = b, c' = c, d' = d, e' = e + 1, label' = "left" }
  action right = {
    nondet r = Set(7, 8).oneOf()
    all { a' = a, b' = b, c' = c, d' = d, e' = r, label' = "right" }
  }
  action group = any { left, right }
  action idle = all { a' = a, b' = b, c' = c, d' = d, e' = e, label' = "idle" }
  action step = any { group, idle }
}
module probe3 {
  var x: int
  action init = x' = 0
  action bump = x' = x + 1
  action step = bump
}
