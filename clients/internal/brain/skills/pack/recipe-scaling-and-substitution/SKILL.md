---
name: recipe-scaling-and-substitution
description: Scale a recipe up or down, convert units (cups, ounces, grams, millilitres, oven temperatures) and suggest safe ingredient substitutions for missing items, allergies or diets, noting how each change affects taste, texture, pan size and cooking time. Use when the user says "make this for 10 people", "halve this recipe", "I have no eggs / buttermilk", "make it dairy-free", "convert cups to grams", or pastes a recipe to adapt.
---

# Recipe scaling and substitution

Cooking scales well; baking is chemistry and needs more care. Always say which changes are safe and which are a gamble.

## Steps
1. **Read the whole recipe.** Note servings, pan size, ingredient amounts, units, oven temperature and time. Ask for the target servings, available equipment, and any allergy or diet.
2. **Compute the factor.** Factor = target servings ÷ original servings. Multiply every ingredient; round to practical amounts (½ egg → beat one egg and use half by weight, about 25 g).
3. **Adjust what does not scale linearly.**
   - Salt, strong spices, chilli: scale by about 0.75 of the factor when going up, then taste.
   - Leavening (baking powder, yeast): scale linearly for small changes; for large batches bake in several pans rather than one huge one.
   - Liquids for reduction or braising: evaporation depends on pan surface, not volume; adjust by eye.
   - Cooking time: depends on thickness, not weight; a bigger pot of soup takes longer to heat, a deeper cake needs a lower temperature and longer time.
4. **Choose the pan.** Compare areas: a 20 cm round pan ≈ 314 cm², a 23 × 33 cm tray ≈ 760 cm². Keep batter depth similar to the original.
5. **Convert units carefully.** Prefer weight for baking. A cup of flour weighs very different amounts depending on how it is filled, so give grams with "about".
6. **Substitute.** Pick a swap that matches the ingredient's job (structure, fat, moisture, acid, leavening, flavour).
7. **Flag food safety.** Allergies need certainty, not approximations: tell the user to read labels for hidden allergens and cross-contamination warnings.

## Common conversions
| Item | Approx. |
|---|---|
| 1 US cup liquid | 240 ml |
| 1 tablespoon / teaspoon | 15 ml / 5 ml |
| 1 oz / 1 lb | 28 g / 454 g |
| 1 cup plain flour | about 120–130 g |
| 1 cup granulated sugar | about 200 g |
| 1 cup butter | about 225 g |
| 180 °C | 350 °F, gas mark 4; fan oven about 160 °C |

## Common substitutions (role → swap)
- Egg as binder: 1 tbsp ground flaxseed + 3 tbsp water, rested 5 min (works in muffins, not in meringue or soufflé).
- Buttermilk: 240 ml milk + 1 tbsp lemon juice or vinegar, rest 5 min.
- Butter in baking: same weight of margarine; oil at about 80 % of the butter weight (texture becomes softer, less flaky).
- Sour cream: thick plain yogurt.
- Self-raising flour: 150 g plain flour + about 1½ tsp baking powder (check the pack guidance).
- Fresh herbs to dried: about one third of the amount.
- Wine in a sauce: stock with a little vinegar or lemon juice.

## Pitfalls
- Scaling bread or cakes by a large factor in one pan; bake in batches.
- Mixing volume and weight in one recipe after conversion.
- Using baking soda for baking powder 1:1; soda is about three to four times stronger and needs acid.
- Gluten-free flour swapped 1:1 in yeast bread; it usually needs a binder and a dedicated recipe.
- Promising an allergy-safe result; the user must check every label.

## Output
Hand back: the full adapted ingredient list with new amounts and units, changed pan size, temperature and time, the substitutions used with their expected effect, and a short note on what to watch or taste while cooking.
