// Phase 0: i64 を超える整数の ITF 表現（Rust バックエンドは QNT601 で停止するため TypeScript バックエンド専用）
module counter_big {
  var big: int
  var neg: int
  action init = all { big' = 2^70, neg' = -(2^70) }
  action step = all { big' = big + 1, neg' = neg - 1 }
}
