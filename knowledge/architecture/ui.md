# Shared look rules

## Lists' filters

How every list's search and filters look (built in `views/lists.templ` and
`listTools` in `static/js/competitions.js`; [how](rendering.md#lists-of-people-search-and-filters)):

- **Counts on each choice.** Every choice of a filter says how many rows it
  would leave, given the search and the other filters chosen: "UCD (12)".
  The first choice ("Every club") counts too. They change as the search and
  filters change, and match what the list would show.
- **Nobody left is greyed out.** A choice that would leave nobody is a
  disabled option, so it can't be chosen by mistake, unless it's the one
  chosen: that stays, with its "(0)", to be seen and taken off.
- **The filters chosen are chips** after "N of M" under the search, in the
  palette's link blue (`tag is-link is-light`, as the Filters button's
  number): "Club: UCD ×". A filter whose choices say what they are on their
  own (`ListFilter.Plain`) is the choice alone: "Not checked ×". The whole
  chip takes its filter off; Clear takes them all.
- The Filters button's number is the number of chips.
- Don't add a second way to show a list's filters; use `ListTools`.
