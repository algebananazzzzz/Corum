# Binary Search

> [!abstract]
> Binary search finds a target in a sorted sequence by halving the search interval after each comparison. Sorting makes each comparison useful: the middle value tells us which half can still contain the target.

## Halving the Search Space

**Search interval** — the portion of the sequence that can still contain the target.

For values sorted in ascending order, compare the target with the middle element:

| Comparison | Meaning | Next interval |
| --- | --- | --- |
| Target < middle | Target belongs before the middle | Left half |
| Target = middle | A match is found | Return its index |
| Target > middle | Target belongs after the middle | Right half |

Each unsuccessful comparison removes the middle element and one half from consideration. ==Binary search preserves every possible match while shrinking the search interval.==

> [!tip] Think of a Dictionary
> Open near the middle and compare the page's words with the word you want. Alphabetical order tells you which side to open next.

## Tracking the Boundaries

Use two indices to describe an inclusive interval, `[low, high]`.

- **Lower bound:** `low` is the first index still under consideration.
- **Upper bound:** `high` is the last index still under consideration.
- **Middle index:** `mid = low + (high - low) // 2` rounds down to an integer.
- **Empty interval:** `low > high` means every candidate has been eliminated.

### Search Procedure

1. **Initialize:** set `low = 0` and `high = len(values) - 1`.
2. **Check interval:** continue while `low <= high`.
3. **Compare middle:** return `mid` when `values[mid]` equals the target.
4. **Keep candidates:** set `high = mid - 1` for a smaller target, or `low = mid + 1` for a larger target.
5. **Repeat:** return to the interval check; report a missing target when the interval becomes empty.

> [!warning] Move Past the Middle
> The comparison has already ruled out `mid`. Updating with `mid - 1` or `mid + 1` guarantees progress, including when a single candidate remains.

## Following One Search

> [!example] Find `23`
> Search `[3, 7, 11, 15, 19, 23, 27]` using zero-based indices.
>
> | Step | `low` | `high` | Middle value | Action |
> | --- | --- | --- | --- | --- |
> | 1 | `0` | `6` | `values[3] = 15` | Keep indices `4–6` |
> | 2 | `4` | `6` | `values[5] = 23` | Return index `5` |
>
> Two comparisons locate the target. The first comparison eliminates indices `0–3` together.

### When the Target Is Missing

Searching the same sequence for `20` first keeps indices `4–6`, then index `4`. Comparing `20` with `values[4] = 19` advances `low` to `5`; `high` remains `4`, so the interval is empty.

> [!example]- Check Your Understanding
> **Question:** which values are compared when searching the sequence above for `7`?
>
> **Answer:** `15`, then `7`. The first comparison keeps indices `0–2`; their middle index is `1`.

## Implementing the Search

This Python implementation accepts an ascending sequence and returns a matching index, or `None` when the target is absent.

```python
def binary_search(values: list[int], target: int) -> int | None:
    low = 0
    high = len(values) - 1

    while low <= high:
        mid = low + (high - low) // 2

        if values[mid] == target:
            return mid
        if target < values[mid]:
            high = mid - 1
        else:
            low = mid + 1

    return None
```

**Invariant** — if the target occurs in the sequence, at least one matching index remains inside `[low, high]` until a match is returned.

- **Initially:** the interval contains every element.
- **After comparison:** sorted order places every possible match in the retained half.
- **At termination:** a returned index identifies a match; an empty interval establishes absence.

> [!warning] Index `0` Is a Valid Result
> Check for a match with `index is not None`. Python treats the valid index `0` as false in a condition such as `if index:`.

## Understanding the Cost

After $k$ unsuccessful comparisons, at most $n / 2^k$ candidates remain, where $n$ is the initial number of elements.

$$
\frac{n}{2^k} \leq 1
\quad\Longrightarrow\quad
k \geq \log_2 n
$$

For $n \geq 1$, binary search examines at most $\lfloor \log_2 n \rfloor + 1$ middle elements. An array of 1,024 elements therefore needs at most 11 comparisons, including the final candidate.

| Dimension | Linear search | Binary search |
| --- | --- | --- |
| Input order | Any | Sorted by the comparison used |
| Worst-case time | $O(n)$ | $O(\log n)$ with constant-time indexing |
| Best-case time | $O(1)$ | $O(1)$ |
| Extra space | $O(1)$ when iterative | $O(1)$ when iterative |
| Suitable use | One lookup in unsorted data | Repeated lookups in a sorted array |

Sorting an unsorted array first typically costs $O(n \log n)$. Repeated searches can justify that setup cost; a single lookup is usually simpler with linear search.

## Handling Boundary Cases

| Case | Result from this implementation |
| --- | --- |
| Empty sequence | Returns `None` immediately |
| Single element | Compares that element once |
| Target outside the value range | Shrinks the interval to empty; returns `None` |
| Repeated target values | Returns one matching index |

> [!warning] Choosing Among Duplicates
> Finding the *first* or *last* occurrence requires a boundary-search variant. This implementation returns as soon as it finds an equal value.
