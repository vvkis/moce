import assert from "node:assert/strict"
import { test } from "node:test"
import type { ProbeEntry } from "../../lib/types"
import { collectBranchIds, groupToc, visibleToc } from "./toc.ts"

function entry(title: string, level: number): ProbeEntry {
  return { title, level, kind: level === 1 ? "chapter" : "chapter", paragraphs: 1, snippet: "" }
}

test("flat book stays a list", () => {
  const tree = groupToc([entry("第一章", 1), entry("第二章", 1), entry("后记", 1)])
  assert.equal(tree.length, 3)
  assert.deepEqual(
    tree.map((node) => node.children.length),
    [0, 0, 0],
  )
  assert.deepEqual(collectBranchIds(tree), [])
})

test("two levels: volume and 番外 parent their chapters", () => {
  const tree = groupToc([
    entry("楔子", 1),
    entry("第一卷 上游", 1),
    entry("第一章 纸灯", 2),
    entry("第二章 旧码头", 2),
    entry("番外 灯芯", 1),
    entry("第一章 余温", 2),
    entry("后记", 1),
  ])
  assert.deepEqual(
    tree.map((node) => node.entry.title),
    ["楔子", "第一卷 上游", "番外 灯芯", "后记"],
  )
  assert.equal(tree[0].children.length, 0)
  assert.deepEqual(
    tree[1].children.map((node) => node.entry.title),
    ["第一章 纸灯", "第二章 旧码头"],
  )
  assert.deepEqual(
    tree[2].children.map((node) => node.entry.title),
    ["第一章 余温"],
  )
  assert.equal(tree[3].children.length, 0)
  assert.deepEqual(collectBranchIds(tree), [1, 4])
})

test("deeper levels follow the level numbers, not the title words", () => {
  const tree = groupToc([
    entry("甲", 1),
    entry("第一卷 并不因此升一层", 2),
    entry("乙", 3),
    entry("丙", 3),
    entry("丁", 2),
    entry("戊", 1),
  ])
  assert.equal(tree.length, 2)
  assert.equal(tree[0].entry.title, "甲")
  assert.equal(tree[0].children.length, 2)
  assert.equal(tree[0].children[0].entry.title, "第一卷 并不因此升一层")
  assert.deepEqual(
    tree[0].children[0].children.map((node) => node.entry.title),
    ["乙", "丙"],
  )
  assert.equal(tree[0].children[1].entry.title, "丁")
  assert.equal(tree[0].children[1].children.length, 0)
  assert.equal(tree[1].entry.title, "戊")
  assert.deepEqual(
    tree.map((node) => node.depth),
    [0, 0],
  )
  assert.equal(tree[0].children[0].depth, 1)
  assert.equal(tree[0].children[0].children[0].depth, 2)
})

test("collapsing a branch hides every descendant", () => {
  const tree = groupToc([
    entry("甲", 1),
    entry("乙", 2),
    entry("丙", 3),
    entry("丁", 2),
    entry("戊", 1),
  ])
  const open = visibleToc(tree, new Set())
  assert.deepEqual(
    open.map((node) => node.entry.title),
    ["甲", "乙", "丙", "丁", "戊"],
  )
  const mid = visibleToc(tree, new Set([1]))
  assert.deepEqual(
    mid.map((node) => node.entry.title),
    ["甲", "乙", "丁", "戊"],
  )
  const shut = visibleToc(tree, new Set(collectBranchIds(tree)))
  assert.deepEqual(
    shut.map((node) => node.entry.title),
    ["甲", "戊"],
  )
})

test("a thousand siblings stay flat and group quickly", () => {
  const entries = Array.from({ length: 1200 }, (_, i) => entry(`第${i + 1}章`, 1))
  const started = Date.now()
  const tree = groupToc(entries)
  assert.ok(Date.now() - started < 500)
  assert.equal(tree.length, 1200)
  assert.equal(collectBranchIds(tree).length, 0)
})
