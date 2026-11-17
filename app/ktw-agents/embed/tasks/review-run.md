---
name: review-run
title: Review run
type: workflow
description: "Run one review type on one object: the review agent follows the type's skill, then the reviews service files its result (the review, its findings, the older reviews folded)"
iconName: clipboard-check
icon: "🔎"
hashtags: [reviews, review, quality]
category: analysis
config:
  steps:
    - name: review
      taskName: review-agent
      input:
        object: "${inputs.object}"
        type: "${inputs.type}"
        title: "${inputs.title}"
        description: "${inputs.description}"
        rating: "${inputs.rating}"
        skill: "${inputs.skill}"
        input: "${inputs.input}"
    - kind: call
      name: file
      uri: ktw-agents:reviews/file
      input:
        object: "${inputs.object}"
        type: "${inputs.type}"
        run: "${inputs.run}"
        trigger: "${inputs.trigger}"
        requested_by: "${inputs.requested_by}"
        commit: "${inputs.commit}"
        reviewers: ["review-agent"]
        result: "${results.review}"
    - kind: return
      value: "${results.file}"
---
One review type on one object, as the reviews service's Start launches it for
a type an agent performs: the `review-agent` task follows the type's skill and
answers a review result; the reviews service's `file` method files it — the
review under the object, each finding matched to the open one with the same
fingerprint or filed new, the ones no longer seen closed — and folds the older
reviews of the type into the object's history.
