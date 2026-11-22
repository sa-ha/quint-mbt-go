// 在庫予約サービスのモデル。
// - 予約要求は受付（submit）と処理（process）の 2 ステップに分かれ、在庫は処理の時点で検証する。
//   2 クライアントの要求が割り込むことで、検証と引き当てを分けた実装の競合をトレースで表せる
// - 確定（confirm）とキャンセル（cancel）は既存の予約すべてに対して試せる。拒否された操作も
//   lastResult に残し、遷移ガードの欠落を観測できるようにする
// - 時刻は論理時刻 now で、tick で 1 進む。期限（expiresAt）が now 以下になった Pending の予約は
//   tick の時点で Expired になり、在庫に戻る
module inventory {
  type Status = Pending | Confirmed | Cancelled | Expired
  type Reservation = { sku: str, qty: int, status: Status, expiresAt: int }
  type Request = { sku: str, qty: int }
  type Outcome = Ok | Rejected(str)

  pure val SKUS = Set("apple", "pear")
  pure val CLIENTS = Set("c1", "c2")
  pure val INITIAL_STOCK = 2
  pure val MAX_QTY = 3
  pure val TTL = 2

  var stock: str -> int
  var reservations: int -> Reservation
  var nextId: int
  var now: int
  var inflight: str -> Request
  var lastResult: Outcome

  action init = all {
    stock' = SKUS.mapBy(_ => INITIAL_STOCK),
    reservations' = Map(),
    nextId' = 1,
    now' = 0,
    inflight' = Map(),
    lastResult' = Ok,
  }

  action submit = {
    nondet client = CLIENTS.exclude(inflight.keys()).oneOf()
    nondet sku = SKUS.oneOf()
    nondet qty = 1.to(MAX_QTY).oneOf()
    all {
      inflight' = inflight.put(client, { sku: sku, qty: qty }),
      lastResult' = Ok,
      stock' = stock,
      reservations' = reservations,
      nextId' = nextId,
      now' = now,
    }
  }

  action process = {
    nondet client = inflight.keys().oneOf()
    val req = inflight.get(client)
    all {
      inflight' = inflight.keys().exclude(Set(client)).mapBy(c => inflight.get(c)),
      now' = now,
      if (stock.get(req.sku) >= req.qty) all {
        stock' = stock.setBy(req.sku, s => s - req.qty),
        reservations' = reservations.put(nextId, { sku: req.sku, qty: req.qty, status: Pending, expiresAt: now + TTL }),
        nextId' = nextId + 1,
        lastResult' = Ok,
      } else all {
        stock' = stock,
        reservations' = reservations,
        nextId' = nextId,
        lastResult' = Rejected("insufficient"),
      },
    }
  }

  action confirm = {
    nondet id = reservations.keys().oneOf()
    val r = reservations.get(id)
    all {
      if (r.status == Pending) all {
        reservations' = reservations.set(id, r.with("status", Confirmed)),
        lastResult' = Ok,
      } else all {
        reservations' = reservations,
        lastResult' = Rejected("notPending"),
      },
      stock' = stock,
      nextId' = nextId,
      now' = now,
      inflight' = inflight,
    }
  }

  action cancel = {
    nondet id = reservations.keys().oneOf()
    val r = reservations.get(id)
    all {
      if (r.status == Pending) all {
        reservations' = reservations.set(id, r.with("status", Cancelled)),
        stock' = stock.setBy(r.sku, s => s + r.qty),
        lastResult' = Ok,
      } else all {
        reservations' = reservations,
        stock' = stock,
        lastResult' = Rejected("notPending"),
      },
      nextId' = nextId,
      now' = now,
      inflight' = inflight,
    }
  }

  action tick = {
    val later = now + 1
    val due = reservations.keys().filter(id => reservations.get(id).status == Pending and reservations.get(id).expiresAt <= later)
    all {
      now' = later,
      reservations' = reservations.keys().mapBy(id =>
        if (due.contains(id)) reservations.get(id).with("status", Expired) else reservations.get(id)),
      stock' = SKUS.mapBy(s => stock.get(s) + due.filter(id => reservations.get(id).sku == s)
        .fold(0, (acc, id) => acc + reservations.get(id).qty)),
      nextId' = nextId,
      inflight' = inflight,
      lastResult' = Ok,
    }
  }

  action step = any { submit, process, confirm, cancel, tick }

  // 在庫は負にならず、在庫と Pending・Confirmed の予約数量の合計は常に初期在庫に等しい
  val stockNonNegative = SKUS.forall(s => stock.get(s) >= 0)
  val conserved = SKUS.forall(s => stock.get(s) + reservations.keys()
    .filter(id => reservations.get(id).sku == s and (reservations.get(id).status == Pending or reservations.get(id).status == Confirmed))
    .fold(0, (acc, id) => acc + reservations.get(id).qty) == INITIAL_STOCK)
}
