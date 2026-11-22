module counter {
  // 状態変数（宣言順を意図的に非アルファベット順にする）
  var zcount: int
  var big: int
  var tags: Set[str]
  var ledger: str -> int
  var pair: (int, str)
  var rec: { name: str, n: int }
  var opt: Option[int]
  var hist: List[int]

  type Option[a] = Some(a) | None

  action init = all {
    zcount' = 0,
    big' = 2^62,
    tags' = Set(),
    ledger' = Map(),
    pair' = (0, "a"),
    rec' = { name: "x", n: 0 },
    opt' = None,
    hist' = [],
  }

  action inc(k: int, who: str): bool = all {
    zcount' = zcount + k,
    big' = big + k,
    tags' = tags.union(Set(who)),
    ledger' = ledger.put(who, k),
    pair' = (k, who),
    rec' = { name: who, n: rec.n + 1 },
    opt' = Some(k),
    hist' = hist.append(k),
  }

  action reset = all {
    zcount' = 0,
    big' = big,
    tags' = Set(),
    ledger' = Map(),
    pair' = pair,
    rec' = rec,
    opt' = None,
    hist' = hist,
  }

  action step = any {
    nondet k = 1.to(3).oneOf()
    nondet who = Set("alice", "bob", "carol").oneOf()
    inc(k, who),
    reset,
  }
}
