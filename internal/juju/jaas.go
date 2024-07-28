// Copyright 2024 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package juju

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	jimmAPI "github.com/kian99/jimm-go-api/v3/api"
	"github.com/kian99/jimm-go-api/v3/api/params"
)

type jaasClient struct {
	SharedClient

	getJimmAPIClient func(connection api.Connection) *jimmAPI.Client
}

func newJaasClient(sc SharedClient) *jaasClient {
	return &jaasClient{
		SharedClient: sc,
		getJimmAPIClient: func(connection api.Connection) *jimmAPI.Client {
			return jimmAPI.NewClient(connection)
		},
	}
}

func (j *jaasClient) AddTuples(tuples []params.RelationshipTuple) error {
	conn, err := j.GetConnection(nil)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	cl := j.getJimmAPIClient(conn)
	req := params.AddRelationRequest{
		Tuples: tuples,
	}
	return cl.AddRelation(&req)
}

func (j *jaasClient) DeleteTuples(tuples []params.RelationshipTuple) error {
	conn, err := j.GetConnection(nil)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	cl := j.getJimmAPIClient(conn)
	req := params.RemoveRelationRequest{
		Tuples: tuples,
	}
	return cl.RemoveRelation(&req)
}

func (j *jaasClient) ReadTuples(tuple params.RelationshipTuple) ([]params.RelationshipTuple, error) {
	conn, err := j.GetConnection(nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	cl := j.getJimmAPIClient(conn)
	req := params.ListRelationshipTuplesRequest{Tuple: tuple}
	tuples := make([]params.RelationshipTuple, 0)
	for {
		response, err := cl.ListRelationshipTuples(&req)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(response.Errors) > 0 {
			return nil, errors.New(response.Errors[0])
		}
		tuples = append(tuples, response.Tuples...)

		if response.ContinuationToken == "" {
			return tuples, nil
		}
		req.ContinuationToken = response.ContinuationToken
	}
}
