package main

import (
	"fmt"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func askTraceCaseDisclosureQuestion(casePath, questionID string) (trace.CaseDisclosureQuestionAnswer, error) {
	round, err := readTraceCaseDisclosureQuestionRound(casePath)
	if err != nil {
		return trace.CaseDisclosureQuestionAnswer{}, err
	}
	for _, answer := range round.Answers {
		if answer.QuestionID == questionID {
			return answer, nil
		}
	}
	return trace.CaseDisclosureQuestionAnswer{}, fmt.Errorf("trace disclosure-map question ID is invalid")
}

func askAllTraceCaseDisclosureQuestions(casePath string) ([]trace.CaseDisclosureQuestionAnswer, error) {
	round, err := readTraceCaseDisclosureQuestionRound(casePath)
	if err != nil {
		return nil, err
	}
	return round.Answers, nil
}

func readTraceCaseDisclosureQuestionRound(casePath string) (trace.CaseDisclosureQuestionRound, error) {
	casePackage, summary, err := trace.ReadCase(casePath)
	if err != nil {
		return trace.CaseDisclosureQuestionRound{}, err
	}
	return trace.AnswerCaseDisclosureQuestionRound(casePackage, summary)
}
