import { describe, expect, it } from "vitest";

import { extractNexusPHPTorrentId } from "./auto-collector";

describe("extractNexusPHPTorrentId", () => {
  const cases: Array<{ name: string; html: string; expected: string | null }> = [
    {
      name: "prefers the free torrent in a table layout",
      html: `
        <table class="torrents">
          <tr><td><a href="details.php?id=10001">First torrent</a></td></tr>
          <tr><td><a href="details.php?id=10002">Free torrent</a><span class='free'>Free</span></td></tr>
        </table>
      `,
      expected: "10002",
    },
    {
      name: "returns the first torrent in a table layout without a free marker",
      html: `
        <table class="torrents">
          <tr><td><a href="details.php?id=20001">First torrent</a></td></tr>
          <tr><td><a href="details.php?id=20002">Second torrent</a></td></tr>
        </table>
      `,
      expected: "20001",
    },
    {
      name: "finds a free torrent in the div layout used by cspt",
      html: `
        <div class="torrents">
          <div class="torrent-cat"><img alt="Movies" /></div>
          <div class="torrent-title">
            <a href="details.php?id=30001&amp;hit=1">First torrent</a>
          </div>
          <div class="torrent-info"><a href="details.php?id=30001&amp;dllist=1">4</a></div>
          <div class="torrent-cat"><img alt="TV" /></div>
          <div class="torrent-title">
            <a href="details.php?id=30002&amp;hit=1">Free torrent</a>
            <img class="pro_free" alt="Free" />
          </div>
        </div>
      `,
      expected: "30002",
    },
    {
      name: "returns the first torrent in a div layout without a free marker",
      html: `
        <div class="torrents">
          <div class="torrent-cat"><img alt="Movies" /></div>
          <div class="torrent-title">
            <a href="details.php?id=40001&amp;hit=1">First torrent</a>
          </div>
          <div class="torrent-title">
            <a href="details.php?id=40002&amp;hit=1">Second torrent</a>
          </div>
        </div>
      `,
      expected: "40001",
    },
    {
      name: "does not treat a userdetails link as a torrent link",
      html: `<table><tr><td><a href="userdetails.php?id=12345">Anonymous user</a></td></tr></table>`,
      expected: null,
    },
    {
      name: "returns null for empty HTML",
      html: "",
      expected: null,
    },
    {
      name: "returns null for empty and no-match HTML",
      html: `<div class="torrents">No torrent links</div>`,
      expected: null,
    },
  ];

  it.each(cases)("$name", ({ html, expected }) => {
    expect(extractNexusPHPTorrentId(html)).toBe(expected);
  });
});
