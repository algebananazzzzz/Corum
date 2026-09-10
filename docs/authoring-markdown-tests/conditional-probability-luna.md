# Conditional Probability

**Conditional probability:** measures how likely an event is after restricting attention to cases where another event is known to occur.

## Restricting the Population

For events (A) and (B),

$$
P(A\mid B)=\frac{P(A\cap B)}{P(B)},\qquad P(B)>0.
$$

The condition (B) becomes the relevant population. The numerator counts cases in both (A) and (B), while the denominator counts all cases satisfying (B). The requirement (P(B)>0) ensures that the restricted population is not empty.

> [!tip] Read the Bar
> In (P(A\mid B)), read the vertical bar as “given.” Start with the event after the bar: it tells you which cases belong in the denominator.

## Applying the Formula

Suppose a school has 200 students. Of these, 80 belong to a science club, 50 study music, and 20 do both.

> [!example] Two Directions
> **Music given club:** restrict the population to the 80 club members. Of those, 20 study music, so (P(\text{music}\mid\text{club})=20/80=25\%\).
>
> **Club given music:** now restrict the population to the 50 music students. Of those, 20 belong to the club, so (P(\text{club}\mid\text{music})=20/50=40\%\).

Reversing the condition changes the denominator, so these probabilities answer different questions even though they share the same intersection of 20 students.

## Interpreting the Result

Conditional probability describes an association under a specified condition. It does not, by itself, establish that club membership causes students to study music, or that studying music causes club membership. A causal claim needs additional evidence and a suitable study design.

> [!warning] Keep the Condition in the Denominator
> For (P(A\mid B)), divide by the probability of (B), not by the total population or by (P(A)). Choosing the wrong denominator silently answers a different question.

==The condition after the bar defines the population you count.==
