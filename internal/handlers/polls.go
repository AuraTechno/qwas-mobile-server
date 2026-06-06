package handlers

import (
	"strconv"
	"time"

	"github.com/AuraTechno/qwas-mobile-server/internal/db"
	"github.com/AuraTechno/qwas-mobile-server/internal/ws"
	"github.com/gofiber/fiber/v2"
)

type PollsHandler struct {
	DB  *db.DB
	Hub *ws.Hub
}

func NewPollsHandler(d *db.DB, hub *ws.Hub) *PollsHandler {
	return &PollsHandler{DB: d, Hub: hub}
}

// GET /api/v1/polls/:id
func (h *PollsHandler) Get(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	pollID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid id"})
	}

	var messageID, chatID int64
	var question string
	var isAnonymous, isMultiple bool
	var closesAt *time.Time
	err = h.DB.Pool.QueryRow(c.Context(), `
		SELECT p.message_id, m.chat_id, p.question, p.is_anonymous, p.is_multiple, p.closes_at
		FROM polls p JOIN messages m ON m.id=p.message_id
		WHERE p.id=$1
	`, pollID).Scan(&messageID, &chatID, &question, &isAnonymous, &isMultiple, &closesAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": "Poll not found"})
	}

	// Check membership
	var isMember bool
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, userID).Scan(&isMember)
	if !isMember {
		return c.Status(403).JSON(fiber.Map{"ok": false, "error": "Forbidden"})
	}

	rows, err := h.DB.Pool.Query(c.Context(), `
		SELECT po.id, po.text, po.sort_order,
		       COALESCE((SELECT COUNT(*) FROM poll_votes pv WHERE pv.option_id=po.id), 0) AS votes
		FROM poll_options po
		WHERE po.poll_id=$1
		ORDER BY po.sort_order
	`, pollID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	defer rows.Close()

	options := []fiber.Map{}
	total := 0
	for rows.Next() {
		var id int64
		var text string
		var order, votes int
		if err := rows.Scan(&id, &text, &order, &votes); err != nil {
			continue
		}
		total += votes
		opt := fiber.Map{"id": id, "text": text, "votes": votes}
		if !isAnonymous {
			voterRows, _ := h.DB.Pool.Query(c.Context(), `SELECT user_id FROM poll_votes WHERE option_id=$1`, id)
			var voters []int64
			for voterRows.Next() {
				var uid int64
				_ = voterRows.Scan(&uid)
				voters = append(voters, uid)
			}
			voterRows.Close()
			opt["voters"] = voters
		}
		options = append(options, opt)
	}

	// My votes
	myVoteRows, _ := h.DB.Pool.Query(c.Context(), `SELECT option_id FROM poll_votes WHERE poll_id=$1 AND user_id=$2`, pollID, userID)
	var myVotes []int64
	for myVoteRows.Next() {
		var oid int64
		_ = myVoteRows.Scan(&oid)
		myVotes = append(myVotes, oid)
	}
	myVoteRows.Close()
	if myVotes == nil {
		myVotes = []int64{}
	}

	return c.JSON(fiber.Map{
		"ok":          true,
		"id":          pollID,
		"messageId":   messageID,
		"question":    question,
		"isAnonymous": isAnonymous,
		"isMultiple":  isMultiple,
		"closesAt":    closesAt,
		"options":     options,
		"totalVotes":  total,
		"myVotes":     myVotes,
	})
}

// POST /api/v1/polls/:id/vote  body: { optionIds: number[] }
func (h *PollsHandler) Vote(c *fiber.Ctx) error {
	userID := c.Locals("userId").(int64)
	pollID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid id"})
	}

	var req struct {
		OptionIDs []int64 `json:"optionIds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": "Invalid body"})
	}

	// Get poll info
	var isMultiple bool
	var messageID, chatID int64
	var closesAt *time.Time
	err = h.DB.Pool.QueryRow(c.Context(), `
		SELECT p.is_multiple, p.message_id, m.chat_id, p.closes_at
		FROM polls p JOIN messages m ON m.id=p.message_id
		WHERE p.id=$1
	`, pollID).Scan(&isMultiple, &messageID, &chatID, &closesAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": "Poll not found"})
	}

	// Check membership
	var isMember bool
	_ = h.DB.Pool.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, userID).Scan(&isMember)
	if !isMember {
		return c.Status(403).JSON(fiber.Map{"ok": false, "error": "Forbidden"})
	}

	// Check closed
	if closesAt != nil && closesAt.Before(time.Now()) {
		return c.JSON(fiber.Map{"ok": false, "error": "Poll closed"})
	}

	if !isMultiple && len(req.OptionIDs) > 1 {
		return c.JSON(fiber.Map{"ok": false, "error": "Single-choice poll"})
	}

	tx, err := h.DB.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}
	defer tx.Rollback(c.Context())

	// Clear previous votes
	_, _ = tx.Exec(c.Context(), `DELETE FROM poll_votes WHERE poll_id=$1 AND user_id=$2`, pollID, userID)

	// Insert new votes
	for _, oid := range req.OptionIDs {
		// Validate option belongs to this poll
		var ownerPoll int64
		_ = tx.QueryRow(c.Context(), `SELECT poll_id FROM poll_options WHERE id=$1`, oid).Scan(&ownerPoll)
		if ownerPoll != pollID {
			continue
		}
		_, err = tx.Exec(c.Context(), `INSERT INTO poll_votes (poll_id, option_id, user_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, pollID, oid, userID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error (vote)"})
		}
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(500).JSON(fiber.Map{"ok": false, "error": "DB error"})
	}

	// Broadcast poll update
	if h.Hub != nil {
		memberRows, _ := h.DB.Pool.Query(c.Context(), `SELECT user_id FROM chat_members WHERE chat_id=$1`, chatID)
		defer memberRows.Close()
		var memberIDs []int64
		for memberRows.Next() {
			var id int64
			_ = memberRows.Scan(&id)
			memberIDs = append(memberIDs, id)
		}
		for _, mid := range memberIDs {
			h.Hub.SendToUser(mid, "poll_updated", fiber.Map{
				"pollId":    pollID,
				"messageId": messageID,
				"chatId":    chatID,
			})
		}
	}

	return c.JSON(fiber.Map{"ok": true})
}
