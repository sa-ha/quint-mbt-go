// Phase 0 の表現確認用。ITF 上の表現と mbt メタデータの記録規則を調べる。
module probe {
  type Kind = Red | Green(int) | Blue({ a: int, b: str })

  var n: int
  var edge: int
  var intKeyed: int -> str
  var tupKeyed: (int, str) -> bool
  var setOfTup: Set[(int, int)]
  var setOfSet: Set[Set[int]]
  var kind: Kind
  var last: str

  action init = all {
    n' = 0,
    edge' = 2^53 - 1,
    intKeyed' = Map(3 -> "c", 1 -> "a", 2 -> "b"),
    tupKeyed' = Map((2, "y") -> true, (1, "x") -> false),
    setOfTup' = Set((3, 1), (1, 2), (2, 9)),
    setOfSet' = Set(Set(3, 1), Set(2), Set()),
    kind' = Red,
    last' = "init",
  }

  // 入れ子のアクション。actionTaken にどちらの名前が残るかを見る
  action innerBump(d: int): bool = all {
    n' = n + d,
    edge' = edge + 1,
    intKeyed' = intKeyed.put(n + d, "x"),
    tupKeyed' = tupKeyed,
    setOfTup' = setOfTup.union(Set((d, n))),
    setOfSet' = setOfSet.union(Set(Set(d))),
    kind' = Green(d),
    last' = "bump",
  }
  action outerBump = {
    nondet d = 1.to(2).oneOf()
    innerBump(d)
  }

  // 別アクションでも同名の nondet を使う
  action paint = {
    nondet d = Set(10, 20).oneOf()
    nondet tag = Set("p", "q").oneOf()
    all {
      n' = n,
      edge' = edge - 1,
      intKeyed' = intKeyed,
      tupKeyed' = tupKeyed.put((d, tag), true),
      setOfTup' = setOfTup,
      setOfSet' = setOfSet,
      kind' = Blue({ a: d, b: tag }),
      last' = "paint",
    }
  }

  // 何もしないアクション（stutter）
  action idle = all {
    n' = n, edge' = edge, intKeyed' = intKeyed, tupKeyed' = tupKeyed,
    setOfTup' = setOfTup, setOfSet' = setOfSet, kind' = kind, last' = "idle",
  }

  action step = any { outerBump, paint, idle }
}
